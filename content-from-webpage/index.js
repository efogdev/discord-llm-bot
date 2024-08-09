import { Readability, isProbablyReaderable } from '@mozilla/readability'
import { JSDOM } from 'jsdom'
import puppeteer from 'puppeteer'

const ONLOAD_TIMEOUT_MS = 5000
const TOTAL_TIMEOUT_MS = 12000 // >= ONLOAD_TIMEOUT

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
        const browser = await puppeteer.launch()

        this.webpage = await browser.newPage()

        try {
            await this.webpage.goto(this.url)

            await new Promise(resolve => {
                this.webpage.once('load', resolve)
                setTimeout(resolve, ONLOAD_TIMEOUT_MS)
            })

            if (await this.checkReadability())
                return this.parse()

            await new Promise(resolve => {
                setTimeout(resolve, Math.max(0, TOTAL_TIMEOUT_MS - ONLOAD_TIMEOUT_MS))
            })

            if (await this.checkReadability())
                return this.parse()

            process.exit(3)
        } catch (e) {
            console.error(e)
            process.exit(2)
        }
    }

    async checkReadability() {
        const documentString = await this.webpage.evaluate(() => new XMLSerializer().serializeToString(document))
        this.document = new JSDOM(documentString).window.document

        return isProbablyReaderable(this.document)
    }

    async parse() {
        if (!this.document)
            return

        const parser = new Readability(this.document)

        if (!parser)
            return

        const { textContent } = parser.parse()

        process.stdout.write(textContent)
        process.exit(0)
    }
}

new WebpageContentParser(process.argv[2])
