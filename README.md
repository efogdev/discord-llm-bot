## Setup

### content-from-webpage 
Navigate to the `content-from-webpage` subdirectory and execute `bun install`

### Bot
Populate `app.env` (see `example.env`)

```shell
go get
go run main.go
```

## Features

### Message History
The bot stores all message history in a local SQLite database. This allows the bot to:
- Keep a persistent record of all conversations
- Retrieve message history from the database instead of fetching from Discord
- Include relevant context from previous conversations when generating responses

You can configure the database path in the `app.env` file using the `DB_PATH` variable. By default, it will create a `messages.db` file in the current directory.
