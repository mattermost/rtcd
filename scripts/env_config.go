// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package main

import (
	"log"
	"os"
	"text/tabwriter"

	"github.com/mattermost/rtcd/service"

	"github.com/kelseyhightower/envconfig"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("unexpected number of arguments, need 1")
	}

	outFile, err := os.OpenFile(os.Args[1], os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		log.Fatalf("failed to write file: %s", err.Error())
	}
	defer outFile.Close()
	if err := outFile.Truncate(0); err != nil {
		log.Fatalf("failed to truncate file: %s", err.Error())
	}
	if _, err := outFile.Seek(0, 0); err != nil {
		log.Fatalf("failed to seek file: %s", err.Error())
	}
	fmt := "### Config Environment Overrides\n\n```\nKEY	TYPE\n{{range .}}{{usage_key .}}	{{usage_type .}}\n{{end}}```\n"
	tabs := tabwriter.NewWriter(outFile, 1, 0, 4, ' ', 0)
	_ = envconfig.Usagef("rtcd", &service.Config{}, tabs, fmt)
	tabs.Flush()
}
