package main

import (
	"context"
	"log"
	"time"

	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/chromedp"
)

func getPageWithChrome(url string) *string {
	log.Println("Chrome is getting:", url)

	// create context
	ctx, cancel := chromedp.NewContext(
		context.Background(),
		chromedp.WithLogf(log.Printf),
	)
	defer cancel()

	// create a timeout as a safety net to prevent any infinite wait loops
	ctx, cancel = context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	html := new(string)

	task := scrapIt(url, html)
	err := chromedp.Run(ctx, task)
	handleChromeErr(err)

	return html
	// return strings.NewReader(*html)
}

func scrapIt(url string, str *string) chromedp.Tasks {
	return chromedp.Tasks{
		chromedp.Navigate(url),
		chromedp.WaitReady(":root"),
		chromedp.ActionFunc(func(ctx context.Context) error {
			node, err := dom.GetDocument().Do(ctx)
			handleChromeErr(err)
			*str, err = dom.GetOuterHTML().WithNodeID(node.NodeID).Do(ctx)
			return err
		}),
	}
}

func handleChromeErr(err error) {
	if err != nil {
		log.Println("CHROME ERROR:", err)
	}
}
