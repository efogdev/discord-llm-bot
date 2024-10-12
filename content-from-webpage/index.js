import { Readability, isProbablyReaderable } from '@mozilla/readability'
import { JSDOM } from 'jsdom'
import puppeteer from 'puppeteer'
import { readPdfText } from 'pdf-text-reader'

const ONLOAD_TIMEOUT_MS = 5000
const TOTAL_TIMEOUT_MS = 12000 // >= ONLOAD_TIMEOUT

const CUSTOM_STRATEGY = {
    // Twitter
    'https://x.com/': async (webpage) => {
        await webpage.waitForSelector('article[role="article"]')

        return webpage.evaluate(() => document.querySelector('[data-testid="tweetText"]').textContent)
    },

    // Telegram
    'https://t.me/': async (webpage) => {
        setTimeout(() => process.exit(4), ONLOAD_TIMEOUT_MS)

        await webpage.waitForSelector('iframe[src*="https://t.me"]')

        const iframeElementHandle = await webpage.$('iframe[src*="https://t.me"]')
        const iframe = await iframeElementHandle.contentFrame()

        await iframe.waitForSelector('.tgme_widget_message_text')

        return iframe.evaluate(() => document.querySelector('.tgme_widget_message_text').textContent)
    },

    // PDF
    'application/pdf': async (webpage) => {
        setTimeout(() => process.exit(4), ONLOAD_TIMEOUT_MS)
        
        return readPdfText({ url: await webpage.evaluate(() => window.location.href) })
    },
}

class WebpageContentParser {
    webpage = null
    document = null

    constructor(url) {
        if (!url || !url.startsWith('https://')) {
            process.exit(1)
        }

        this.url = url
        this.init()
    }

    async init() {
        const browser = await puppeteer.launch({ headless: true, defaultViewport: { width: 1700, height: 800 } })
        this.webpage = await browser.newPage()

        await this.webpage.setUserAgent('Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/66.0.3359.181 Safari/537.36')

        try {
            const response = await this.webpage.goto(this.url)
            const contentType = response.headers()['content-type']

            if (CUSTOM_STRATEGY[contentType]) {
                return this.parse(contentType)
            }

            await new Promise(resolve => {
                this.webpage.once('load', resolve)
                setTimeout(resolve, ONLOAD_TIMEOUT_MS)
            })

            if (await this.checkReadability())
                return this.parse(contentType)

            await new Promise(resolve => {
                setTimeout(resolve, Math.max(0, TOTAL_TIMEOUT_MS - ONLOAD_TIMEOUT_MS))
            })

            if (await this.checkReadability())
                return this.parse(contentType)

            process.exit(3)
        } catch (e) {
            console.error(e)
            process.exit(2)
        }
    }

    async checkReadability() {
        const url = await this.webpage.evaluate(() => window.location.href)

        if (Object.keys(CUSTOM_STRATEGY).some(key => url.startsWith(key)))
            return true

        const documentString = await this.webpage.evaluate(() => new XMLSerializer().serializeToString(document))
        this.document = new JSDOM(documentString).window.document

        return isProbablyReaderable(this.document)
    }

    async parse(mimeType = 'application/octet-stream') {
        const url = await this.webpage.evaluate(() => window.location.href)
        let content = ''

        const strategy = Object.keys(CUSTOM_STRATEGY).find(key => url.startsWith(key) || key === mimeType)
        if (strategy) {
            content = await CUSTOM_STRATEGY[strategy](this.webpage)
        } else {
            if (!this.document)
                return

            const parser = new Readability(this.document)

            if (!parser)
                return

            content = parser.parse().textContent
        }


        process.stdout.write(content)
        process.exit(0)
    }
}

new WebpageContentParser(process.argv[2])
