package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/nuveo/anticaptcha"
	"github.com/optinsoft/go-chromedp-test/testlib"
)

func getErrorMsg(ctx context.Context) (string, error) {
	var errorMsg *runtime.RemoteObject
	if err := chromedp.Run(ctx,
		chromedp.EvaluateAsDevTools(
			`(function(){var p=document.querySelector('p[class*="error-msg"]');return p&&p.getAttribute('data-error')+': '+p.innerText;})()`,
			&errorMsg,
		),
	); err != nil {
		return "", err
	}
	if errorMsg.Type == "string" {
		m, err := strconv.Unquote(string(errorMsg.Value))
		if err != nil {
			return "", err
		}
		return m, nil
	}
	return "", nil
}

func skipRemindMeLater(ctx context.Context) (bool, error) {
	var remindMeLaterLink *runtime.RemoteObject
	if err := chromedp.Run(ctx,
		chromedp.EvaluateAsDevTools(
			`(function(){var a=document.querySelector('a[href*="https://guce.yahoo.com/consent"]');if(a)a.click();return a&&a.href;})()`,
			&remindMeLaterLink,
		),
	); err != nil {
		return false, err
	}
	return (remindMeLaterLink.Type != "undefined"), nil
}

func checkComposeButton(ctx context.Context) (bool, error) {
	errorMsg, err := getErrorMsg(ctx)
	if err != nil {
		return false, err
	}
	if errorMsg != "" {
		return false, errors.New(errorMsg)
	}
	remindMeLater, err := skipRemindMeLater(ctx)
	if err != nil {
		return false, err
	}
	if remindMeLater {
		if err := chromedp.Run(ctx, chromedp.Sleep(3*time.Second)); err != nil {
			return false, err
		}
	}
	var composeButton *runtime.RemoteObject
	if err := chromedp.Run(ctx,
		chromedp.EvaluateAsDevTools(
			`document.querySelector('a[data-test-id="compose-button"]')?.href`,
			&composeButton,
		),
	); err != nil {
		return false, err
	}
	return (composeButton.Type != "undefined"), nil
}

func needChangePassword(ctx context.Context) (bool, error) {
	var changepasswordLink *runtime.RemoteObject
	if err := chromedp.Run(ctx,
		chromedp.EvaluateAsDevTools(
			`(function(){var a=document.querySelector('a[href*="account/change-password"]');if(a)a.click();return a&&a.href;})()`,
			&changepasswordLink,
		),
	); err != nil {
		return false, err
	}
	return (changepasswordLink.Type != "undefined"), nil
}

func main() {
	var options testlib.TestOptions

	err := testlib.LoadOptions(&options, "yahoo", "YCookies")
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

	if err := testlib.RunWithTimeout(pageCtx, 10*time.Second,
		chromedp.Navigate("https://login.yahoo.com/?.src=ym&.done=https%3A%2F%2Fmail.yahoo.com%2Fd%2F"),
		chromedp.WaitVisible("#login-username", chromedp.ByID),
	); err != nil {
		log.Fatal(err)
	}

	var recaptchaChallenge *runtime.RemoteObject
	if err := testlib.RunWithTimeout(pageCtx, 25*time.Second,
		chromedp.Sleep(time.Second*3),
		chromedp.SendKeys("#login-username", options.User, chromedp.ByID),
		chromedp.Sleep(time.Second*3),
		chromedp.Click("#login-signin", chromedp.NodeVisible),
		chromedp.Sleep(time.Second*3),
		chromedp.EvaluateAsDevTools(
			`document.querySelector('#recaptcha-iframe')?.src`,
			&recaptchaChallenge,
		),
	); err != nil {
		log.Fatal(err)
	}
	if recaptchaChallenge.Type == "string" {
		if len(options.AntiCaptchaApiKey) == 0 {
			log.Fatal(errors.New("ReCaptcha challenge"))
		}
		recaptchaChallengeSrc, err := strconv.Unquote(string(recaptchaChallenge.Value))
		if err != nil {
			log.Fatal(err)
		}
		if strings.HasPrefix(recaptchaChallengeSrc, "/") {
			recaptchaChallengeSrc = "https://login.yahoo.com" + recaptchaChallengeSrc
		}
		u, err := url.Parse(recaptchaChallengeSrc)
		if err != nil {
			//log.Fatal(err)
			log.Fatalf("ReCaptcha challenge src: %s", recaptchaChallengeSrc)
		}
		qp := u.Query()
		siteKey := qp.Get("siteKey")
		client := &anticaptcha.Client{APIKey: options.AntiCaptchaApiKey}
		key, err := client.SendRecaptcha(
			recaptchaChallengeSrc,
			siteKey,
			time.Duration(options.AntiCaptchaTimeoutSeconds)*time.Second,
		)
		if err != nil {
			log.Fatal(err)
		}
		//TODO: resolve ReCaptcha
		log.Fatalf(
			"ReCaptcha challenge siteKey: %s, anti-captcha key: %s",
			siteKey, key,
		)
	}

	if err := testlib.RunWithTimeout(pageCtx, 25*time.Second,
		chromedp.WaitVisible("#login-passwd", chromedp.ByID),
	); err != nil {
		log.Fatal(err)
	}

	if err := testlib.RunWithTimeout(pageCtx, 25*time.Second,
		chromedp.SendKeys("#login-passwd", options.Password, chromedp.ByID),
		chromedp.Sleep(time.Second*3),
		chromedp.Click("#login-signin", chromedp.NodeVisible),
		chromedp.Sleep(time.Second*3),
	); err != nil {
		log.Fatal(err)
	}
	errorMsg, err := getErrorMsg(pageCtx)
	if err != nil {
		log.Fatal(err)
	}
	if errorMsg != "" {
		log.Fatal(errors.New(errorMsg))
	}
	composeButton, err := checkComposeButton(pageCtx)
	if err != nil {
		log.Fatal(err)
	}
	if !composeButton {
		changePassword, err := needChangePassword(pageCtx)
		if err != nil {
			log.Fatal(err)
		}
		if changePassword {
			if len(options.NewPassword) == 0 {
				log.Fatal(errors.New("change password required"))
			}
			if err := testlib.RunWithTimeout(pageCtx, 25*time.Second,
				chromedp.WaitVisible("#cpwd-password", chromedp.ByID),
				chromedp.SendKeys("#cpwd-password", options.NewPassword, chromedp.ByID),
				chromedp.Sleep(time.Second*3),
				chromedp.Click("#ch-pwd-submit-btn", chromedp.NodeVisible),
				chromedp.Sleep(time.Second*5),
			); err != nil {
				log.Fatal(err)
			}
			errorMsg, err := getErrorMsg(pageCtx)
			if err != nil {
				log.Fatal(err)
			}
			if errorMsg != "" {
				log.Fatal(errors.New(errorMsg))
			}
			if err := testlib.RunWithTimeout(pageCtx, 25*time.Second,
				chromedp.WaitVisible("#change-password-success"),
				chromedp.Click("#change-password-success", chromedp.NodeVisible),
				chromedp.Sleep(time.Second*3),
			); err != nil {
				log.Fatal(err)
			}
		}
		composeButton, err = checkComposeButton(pageCtx)
		if err != nil {
			log.Fatal(err)
		}
	}
	if !composeButton {
		log.Fatal(errors.New("compose button not found"))
	}

	log.Printf("Completed. Press any key to stop.")
	fmt.Scanln()
}
