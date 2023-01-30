package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/optinsoft/go-chromedp-test/testlib"
)

func skipDontExit(ctx context.Context) (bool, error) {
	var backButton *runtime.RemoteObject
	if err := chromedp.Run(ctx,
		chromedp.EvaluateAsDevTools(
			`(function(){var a=document.querySelector('#idBtn_Back');if(a)a.click();return a&&a.id;})()`,
			&backButton,
		),
	); err != nil {
		return false, err
	}
	return (backButton.Type != "undefined"), nil
}

func main() {
	var options testlib.TestOptions

	err := testlib.LoadOptions(&options, "hotmail", "HotCookies")
	if err != nil {
		log.Fatal(err)
	}

	dir, err := testlib.TempDir(&options)
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)

	opts, err := testlib.DefaultOpts(&options, dir)
	if err != nil {
		log.Fatal(err)
	}

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	pageCtx, cancel := chromedp.NewContext(
		allocCtx,
		chromedp.WithLogf(log.Printf))
	defer cancel()

	if err := chromedp.Run(pageCtx); err != nil {
		log.Fatal(err)
	}

	if err := chromedp.Run(pageCtx,
		chromedp.Navigate("https://mail.live.com/"),
		chromedp.Click(`//a[@data-task="signin"]`),
		chromedp.WaitVisible(`//input[@name="loginfmt"]`),
		chromedp.Sleep(time.Second*2),
		chromedp.SendKeys(`//input[@name="loginfmt"]`, options.User),
		chromedp.Sleep(time.Second),
		chromedp.Click(`//input[@data-report-value="Submit"]`),
		chromedp.WaitVisible(`//input[@name="passwd"]`),
		chromedp.Sleep(time.Second),
		chromedp.SendKeys(`//input[@name="passwd"]`, options.Password),
		chromedp.Sleep(time.Second*2),
		chromedp.Submit(`//input[@name="passwd"]`),
		chromedp.Sleep(time.Second*3),
	); err != nil {
		log.Fatal(err)
	}

	skip, err := skipDontExit(pageCtx)
	if skip {
		if err := chromedp.Run(pageCtx,
			chromedp.Sleep(time.Second*3),
		); err != nil {
			log.Fatal(err)
		}
	}

	log.Printf("Completed. Press any key to stop.")
	fmt.Scanln()
}
