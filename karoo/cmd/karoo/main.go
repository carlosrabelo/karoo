// Karoo (Go) - Stratum V1 Proxy
// Author: Carlos Rabelo <contato@carlosrabelo.com.br>

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
)

var (
	version   = "dev"
	buildTime = "unknown"
)

func main() {
	cfgFile := flag.String("config", "config.json", "Path to configuration file")
	showVersion := flag.Bool("version", false, "Show version information")
	flag.Parse()

	if *showVersion {
		fmt.Printf("karoo %s (built %s)\n", version, buildTime)
		os.Exit(0)
	}

	if err := run(*cfgFile); err != nil {
		log.Fatal(err)
	}
}
