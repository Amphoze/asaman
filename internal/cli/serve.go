package cli

import (
	"fmt"
	"os"

	"github.com/amphoze/asaman/internal/web"
)

func runServe(cfgPath string, args []string) int {
	port, _, _ := flagVal(args, "port")
	if port == "" {
		port = "7392"
	}
	app, err := loadApp(cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "serve:", err)
		return 1
	}
	db, err := app.openDB()
	if err != nil {
		fmt.Fprintln(os.Stderr, "serve:", err)
		return 1
	}
	defer db.Close()
	ads, _ := app.adapters()
	facts, _ := app.facts()
	if err := web.Serve("127.0.0.1:"+port, db, ads, facts, app.Cfg.MetaStore, app.Lock); err != nil {
		fmt.Fprintln(os.Stderr, "serve:", err)
		return 1
	}
	return 0
}
