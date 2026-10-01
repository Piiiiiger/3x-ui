// Command pigger-agent runs Xray for one Pigger panel node: it dials the panel,
// runs the config the panel sends and reports traffic and load back.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/mhsanaei/3x-ui/v3/internal/agent"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/util/sys"

	"github.com/op/go-logging"
)

// version is set at build time: -ldflags "-X main.version=...".
var version = "dev"

func main() {
	configPath := flag.String("config", "/etc/pigger-agent/config.json", "agent settings file")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}

	logger.InitLogger(logging.INFO)
	for _, line := range sys.ApplyMemoryTuning() {
		logger.Info(line)
	}
	cfg, err := agent.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("pigger-agent: %v", err)
	}
	a, err := agent.New(cfg, version)
	if err != nil {
		log.Fatalf("pigger-agent: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	logger.Info("pigger-agent ", version, " starting")
	a.Run(ctx)
	logger.Info("pigger-agent stopped")
}
