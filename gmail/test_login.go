package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/chromedp/chromedp"
	_ "github.com/mattn/go-sqlite3"

	"github.com/optinsoft/go-chromedp-test/testlib"
)

func main() {
	var options testlib.TestOptions

	err := testlib.LoadOptions(&options, "gmail", "GCookies")
	if err != nil {
		log.Fatal(err)
	}

	dir, err := testlib.TempDir(&options)
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)

	var sqlite_db *sql.DB
	if options.SqliteDbPath != "" {
		sqlite_db, err = sql.Open("sqlite3", options.SqliteDbPath)
		if err != nil {
			log.Fatal(err)
		}
	}

	opts, err := testlib.DefaultOpts(&options, dir)
	if err != nil {
		log.Fatal(err)
	}

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	taskCtx, cancel := chromedp.NewContext(
		allocCtx,
		chromedp.WithLogf(log.Printf))
	defer cancel()

	//taskCtx, cancel := context.WithTimeout(newCtx, 15*time.Second)
	//defer cancel()

	if err := chromedp.Run(taskCtx,
		chromedp.Navigate("https://mail.google.com/"),
		chromedp.WaitVisible("#identifierId", chromedp.ByID),
		//chromedp.SetValue("#identifierId", account.Name, chromedp.ByID),
		chromedp.SendKeys("#identifierId", options.User, chromedp.ByID),
		chromedp.Sleep(time.Second*3),
		//chromedp.Submit("#identifierId", chromedp.ByID),
		chromedp.Click("#identifierNext", chromedp.NodeVisible),
		chromedp.WaitVisible(`//input[@name="Passwd"]`),
		chromedp.SendKeys(`//input[@name="Passwd"]`, options.Password),
		chromedp.Sleep(time.Second*3),
		chromedp.Click("#passwordNext", chromedp.NodeVisible),
		chromedp.WaitVisible(`//a[contains(@href,"#inbox")]`),
		//		chromedp.ActionFunc(func(ctx context.Context) error {
		//			if sqlite_db == nil {
		//				return nil
		//			}
		//			return testlib.LogCookies(ctx, *sqlite_db, options.SqliteCookieTable)
		//		}),
	); err != nil {
		log.Fatal(err)
	}

	if err := testlib.LogCookies(taskCtx, *sqlite_db, options.SqliteCookieTable); err != nil {
		log.Fatal(err)
	}

	log.Printf("Completed. Press any key to stop.")
	fmt.Scanln()
}
