package testlib

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"sort"
	"time"

	"github.com/chromedp/cdproto"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"gopkg.in/ini.v1"
)

type TestOptions struct {
	AccountType               string
	User                      string
	Password                  string
	NewPassword               string
	UserAgent                 string
	Proxy                     string
	SqliteDbPath              string
	SqliteCookieTable         string
	AntiCaptchaApiKey         string
	AntiCaptchaTimeoutSeconds int
}

func LoadOptions(options *TestOptions, accountType string, sqliteCookieTable string) error {

	options.AccountType = accountType

	uaPtr := flag.String("ua", "", "user agent")
	proxyPtr := flag.String("proxy", "", "proxy_host:proxy_port")
	userPtr := flag.String("user", "", "account name")
	passwordPtr := flag.String("password", "", "account password")

	flag.Parse()

	options.User = *userPtr
	options.Password = *passwordPtr
	options.UserAgent = *uaPtr
	options.Proxy = *proxyPtr

	cfg, err := LoadConfig(options)
	if err != nil {
		return err
	}

	accountSection := accountType
	browserSection := "browser"
	sqliteSection := "sqlite"
	anticaptchaSection := "anticaptcha"

	if options.User == "" {
		options.User = cfg.Section(accountSection).Key("user").String()
	}
	if options.Password == "" {
		options.Password = cfg.Section(accountSection).Key("password").String()
	}
	options.NewPassword = cfg.Section(accountSection).Key("newPassword").String()
	if options.UserAgent == "" {
		options.UserAgent = cfg.Section(browserSection).Key("userAgent").String()
	}
	if options.Proxy == "" {
		options.Proxy = cfg.Section(browserSection).Key("proxy").String()
	}

	options.SqliteDbPath = cfg.Section(sqliteSection).Key("dbPath").String()

	options.SqliteCookieTable = cfg.Section(accountSection).Key("cookieTable").String()
	if options.SqliteCookieTable == "" {
		options.SqliteCookieTable = sqliteCookieTable
	}

	options.AntiCaptchaApiKey = cfg.Section(anticaptchaSection).Key("apiKey").String()
	options.AntiCaptchaTimeoutSeconds, err = cfg.Section(anticaptchaSection).Key("timeoutSeconds").Int()
	if err != nil {
		options.AntiCaptchaTimeoutSeconds = 300
	}

	return nil
}

func TempDir(_ *TestOptions) (string, error) {
	dir, err := ioutil.TempDir("", "chromedp-test")
	if err == nil {
		log.Printf("temp dir: %s", dir)
	}
	return dir, err
}

func DefaultOpts(options *TestOptions, dir string) ([]chromedp.ExecAllocatorOption, error) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.DisableGPU,
		chromedp.UserDataDir(dir),
		chromedp.Flag("headless", false),
		chromedp.Flag("enable-automation", false),
		chromedp.Flag("remote-debugging-port", "9222"),
	)

	if options.UserAgent != "" {
		opts = append(opts, chromedp.UserAgent(options.UserAgent))
	}
	if options.Proxy != "" {
		opts = append(opts, chromedp.ProxyServer(fmt.Sprintf("http://%s", options.Proxy)))
	}

	return opts, nil
}

func LoadConfig(options *TestOptions) (*ini.File, error) {
	return ini.LoadSources(ini.LoadOptions{IgnoreInlineComment: true}, "config.ini")
}

func LogCookies(ctx context.Context, db sql.DB, cookieTableName string) error {
	cookies, err := network.GetAllCookies().Do(ctx)
	if err != nil {
		return err
	}
	sort.SliceStable(cookies, func(i int, j int) bool {
		return cookies[i].Domain < cookies[j].Domain
	})
	var newDomain bool = true
	var currentDomain string = ""
	for _, cookie := range cookies {
		if cookie.Domain != currentDomain {
			newDomain = true
			currentDomain = cookie.Domain
		}
		if newDomain {
			log.Printf("[%s]", currentDomain)
			newDomain = false
		}
		log.Printf("chrome cookie: %+v", cookie)
	}
	stmt, err := db.Prepare(fmt.Sprintf("INSERT INTO %s(Login, Cookies) VALUES (?, ?)", cookieTableName))
	if err != nil {
		return err
	}
	res, err := stmt.Exec("test", "cookies")
	if err != nil {
		return err
	}
	affect, err := res.RowsAffected()
	if err != nil {
		return err
	}
	fmt.Printf("Rows inserted: %d", affect)
	return nil
}

func RunWithTimeout(ctx context.Context, timeout time.Duration, actions ...chromedp.Action) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return chromedp.Run(timeoutCtx, actions...)
}

func WaitSelectorReady(s *chromedp.Selector, check func(context.Context, runtime.ExecutionContextID, *cdp.Node) error) func(context.Context, *cdp.Frame, runtime.ExecutionContextID, ...cdp.NodeID) ([]*cdp.Node, error) {
	return func(ctx context.Context, cur *cdp.Frame, execCtx runtime.ExecutionContextID, ids ...cdp.NodeID) ([]*cdp.Node, error) {
		nodes := make([]*cdp.Node, len(ids))
		cur.RLock()
		for i, id := range ids {
			nodes[i] = cur.Nodes[id]
			if nodes[i] == nil {
				cur.RUnlock()
				// not yet ready
				return nil, nil
			}
		}
		cur.RUnlock()

		if check != nil {
			errc := make(chan error, 1)
			for _, n := range nodes {
				go func(n *cdp.Node) {
					select {
					case <-ctx.Done():
						errc <- ctx.Err()
					case errc <- check(ctx, execCtx, n):
					}
				}(n)
			}

			var first error
			for range nodes {
				if err := <-errc; first == nil {
					first = err
				}
			}
			close(errc)
			if first != nil {
				return nil, first
			}
		}
		return nodes, nil
	}
}

func isCouldNotComputeBoxModelError(err error) bool {
	e, ok := err.(*cdproto.Error)
	return ok && e.Code == -32000 && e.Message == "Could not compute box model."
}

func callFunctionOnNode(ctx context.Context, node *cdp.Node, function string, res interface{}, args ...interface{}) error {
	r, err := dom.ResolveNode().WithNodeID(node.NodeID).Do(ctx)
	if err != nil {
		return err
	}
	err = chromedp.CallFunctionOn(function, &res,
		func(p *runtime.CallFunctionOnParams) *runtime.CallFunctionOnParams {
			return p.WithObjectID(r.ObjectID)
		},
		args...,
	).Do(ctx)

	if err != nil {
		return err
	}

	// Try to release the remote object.
	// It will fail if the page is navigated or closed,
	// and it's okay to ignore the error in this case.
	_ = runtime.ReleaseObject(r.ObjectID).Do(ctx)

	return nil
}

func NodeVisibleOrRecaptcha(s *chromedp.Selector) {
	chromedp.WaitFunc(WaitSelectorReady(s, func(ctx context.Context, execCtx runtime.ExecutionContextID, n *cdp.Node) error {
		// check box model
		_, err := dom.GetBoxModel().WithNodeID(n.NodeID).Do(ctx)
		if err != nil {
			if isCouldNotComputeBoxModelError(err) {
				return chromedp.ErrNotVisible
			}
			return err
		}

		// check visibility
		var res bool
		err = callFunctionOnNode(ctx, n, visibleJS, &res)
		if err != nil {
			return err
		}
		if !res {
			return chromedp.ErrNotVisible
		}
		return nil
	}))(s)
}

func WaitVisibleOrRecaptcha(sel interface{}, opts ...chromedp.QueryOption) chromedp.QueryAction {
	return chromedp.Query(sel, append(opts, NodeVisibleOrRecaptcha)...)
}
