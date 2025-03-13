package db

import (
	"database/sql"
	"discord-military-analyst-bot/internal/llm"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/bwmarrin/discordgo"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
)

type MessageDB struct {
	db *sql.DB
}

// Message represents a message stored in the database
type Message struct {
	ID           string
	ChannelID    string
	AuthorID     string
	Content      string
	IsBotMessage bool
	Attachments  string // JSON encoded attachments
	ReferencedID string // ID of the referenced message, if any
	CreatedAt    time.Time
}

// New creates a new MessageDB instance
func New(dbPath string) (*MessageDB, error) {
	if dbPath == "" {
		dbPath = filepath.Join(".", "messages.db")
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	// Create tables if they don't exist
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS messages (
			id TEXT PRIMARY KEY,
			channel_id TEXT NOT NULL,
			author_id TEXT NOT NULL,
			content TEXT NOT NULL,
			is_bot_message BOOLEAN NOT NULL,
			attachments TEXT,
			referenced_id TEXT,
			created_at TIMESTAMP NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_messages_channel_id ON messages(channel_id);
		CREATE INDEX IF NOT EXISTS idx_messages_referenced_id ON messages(referenced_id);
	`)
	if err != nil {
		db.Close()
		return nil, err
	}

	return &MessageDB{db: db}, nil
}

// Close closes the database connection
func (m *MessageDB) Close() error {
	return m.db.Close()
}

// SaveMessage saves a message to the database
func (m *MessageDB) SaveMessage(msg *discordgo.Message, isBotMessage bool) error {
	attachmentsJSON, err := json.Marshal(msg.Attachments)
	if err != nil {
		return err
	}

	var referencedID string
	if msg.ReferencedMessage != nil {
		referencedID = msg.ReferencedMessage.ID
	}

	_, err = m.db.Exec(
		`INSERT OR REPLACE INTO messages 
		(id, channel_id, author_id, content, is_bot_message, attachments, referenced_id, created_at) 
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.ID,
		msg.ChannelID,
		msg.Author.ID,
		msg.Content,
		isBotMessage,
		string(attachmentsJSON),
		referencedID,
		time.Now(),
	)
	return err
}

// GetMessage retrieves a message from the database by ID
func (m *MessageDB) GetMessage(id string) (*Message, error) {
	var msg Message
	err := m.db.QueryRow(
		`SELECT id, channel_id, author_id, content, is_bot_message, attachments, referenced_id, created_at 
		FROM messages WHERE id = ?`,
		id,
	).Scan(
		&msg.ID,
		&msg.ChannelID,
		&msg.AuthorID,
		&msg.Content,
		&msg.IsBotMessage,
		&msg.Attachments,
		&msg.ReferencedID,
		&msg.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("message not found: %s", id)
		}
		return nil, err
	}
	return &msg, nil
}

// GetMessageHistory retrieves the conversation history for a message
func (m *MessageDB) GetMessageHistory(messageID string, botID string) ([]llm.HistoryItem, error) {
	var history []llm.HistoryItem
	var currentID = messageID

	// Track visited messages to avoid infinite loops
	visited := make(map[string]bool)

	for currentID != "" && !visited[currentID] {
		visited[currentID] = true

		msg, err := m.GetMessage(currentID)
		if err != nil {
			// If message not found in DB, break the chain
			zap.L().Debug("message not found in DB", zap.String("id", currentID))
			break
		}

		var attachments []*discordgo.MessageAttachment
		if msg.Attachments != "" {
			if err := json.Unmarshal([]byte(msg.Attachments), &attachments); err != nil {
				zap.L().Error("failed to unmarshal attachments", zap.Error(err))
			}
		}

		history = append([]llm.HistoryItem{{
			IsBotMessage: msg.IsBotMessage,
			Content:      msg.Content,
			Attachments:  attachments,
		}}, history...)

		currentID = msg.ReferencedID
	}

	return history, nil
}

// GetAllRelatedMessages retrieves all messages in the conversation thread
func (m *MessageDB) GetAllRelatedMessages(messageID string, botID string) ([]llm.HistoryItem, error) {
	// First get the direct reply chain
	history, err := m.GetMessageHistory(messageID, botID)
	if err != nil {
		return nil, err
	}

	// Get the channel ID from the first message in history
	var channelID string
	if len(history) > 0 {
		// We need to query the DB to get the channel ID
		msg, err := m.GetMessage(messageID)
		if err != nil {
			return history, nil // Return what we have if we can't get the channel
		}
		channelID = msg.ChannelID
	} else {
		return history, nil // No history found
	}

	// Get recent messages from the same channel (limited to last 50)
	rows, err := m.db.Query(
		`SELECT id, content, is_bot_message, attachments 
		FROM messages 
		WHERE channel_id = ? 
		ORDER BY created_at DESC LIMIT 50`,
		channelID,
	)
	if err != nil {
		return history, nil // Return what we have if query fails
	}
	defer rows.Close()

	// Track IDs we already have in history to avoid duplicates
	existingIDs := make(map[string]bool)
	for _, item := range history {
		// We don't have the ID in the history item, so this is a best-effort
		// to avoid duplicates based on content
		existingIDs[item.Content] = true
	}

	var additionalHistory []llm.HistoryItem
	for rows.Next() {
		var id, content string
		var isBotMessage bool
		var attachmentsJSON string

		if err := rows.Scan(&id, &content, &isBotMessage, &attachmentsJSON); err != nil {
			continue
		}

		// Skip if we already have this content in history
		if existingIDs[content] {
			continue
		}
		existingIDs[content] = true

		var attachments []*discordgo.MessageAttachment
		if attachmentsJSON != "" {
			if err := json.Unmarshal([]byte(attachmentsJSON), &attachments); err != nil {
				zap.L().Error("failed to unmarshal attachments", zap.Error(err))
			}
		}

		additionalHistory = append(additionalHistory, llm.HistoryItem{
			IsBotMessage: isBotMessage,
			Content:      content,
			Attachments:  attachments,
		})
	}

	// Combine the histories, with direct reply chain first
	return append(history, additionalHistory...), nil
}
