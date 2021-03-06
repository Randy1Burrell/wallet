package main

import (
	"github.com/kittycash/wallet/src/http"
	"github.com/kittycash/wallet/src/iko"
	"github.com/kittycash/wallet/src/wallet"
	"github.com/skycoin/skycoin/src/cipher"
	"gopkg.in/sirupsen/logrus.v1"
	"gopkg.in/urfave/cli.v1"
	"os"
	"os/signal"
)

const (
	MasterPublicKey = "master-public-key"

	MemoryMode = "memory"

	TestMode           = "test"
	TestSecretKey      = "test-secret-key"
	TestInjectionCount = "test-injection-count"

	HttpAddress = "http-address"
	GUI         = "gui"
	GUIDir      = "gui-dir"
	TLS         = "tls"
	TLSCert     = "tls-cert"
	TLSKey      = "tls-key"
)

func Flag(flag string, short ...string) string {
	if len(short) == 0 {
		return flag
	}
	return flag + ", " + short[0]
}

var (
	app = cli.NewApp()
	log = logrus.New()
)

func init() {
	app.Name = "iko"
	app.Description = "kittycash initial coin offering service"
	app.Flags = cli.FlagsByName{
		/*
			<<< MASTER PUBLIC KEY >>>
		*/
		cli.StringFlag{
			Name:  Flag(MasterPublicKey, "pk"),
			Usage: "public key to trust as master decision maker",
		},
		/*
			<<< MEMORY MODE >>>
		*/
		cli.BoolFlag{
			Name:  Flag(MemoryMode, "m"),
			Usage: "whether to run in memory-only mode",
		},
		/*
			<<< TEST MODE >>>
		*/
		cli.BoolFlag{
			Name:  Flag(TestMode, "t"),
			Usage: "whether to use test data for run",
		},
		cli.StringFlag{
			Name:  Flag(TestSecretKey, "sk"),
			Usage: "only valid in test mode, used for injecting transactions",
		},
		cli.IntFlag{
			Name:  Flag(TestInjectionCount, "tc"),
			Usage: "only valid in test mode, injects a number of initial transactions for testing",
		},
		/*
			<<< HTTP SERVER >>>
		*/
		cli.StringFlag{
			Name:  Flag(HttpAddress),
			Usage: "address to serve http server on",
			Value: "127.0.0.1:8080",
		},
		cli.BoolTFlag{
			Name:  Flag(GUI),
			Usage: "whether to enable gui",
		},
		cli.StringFlag{
			Name:  Flag(GUIDir),
			Usage: "directory to serve GUI from",
			Value: "./static",
		},
		cli.BoolFlag{
			Name:  Flag(TLS),
			Usage: "whether to enable tls",
		},
		cli.StringFlag{
			Name:  Flag(TLSCert),
			Usage: "tls certificate file path",
		},
		cli.StringFlag{
			Name:  Flag(TLSKey),
			Usage: "tls key file path",
		},
	}
	app.Action = cli.ActionFunc(action)
}

func action(ctx *cli.Context) error {
	quit := CatchInterrupt()

	var (
		masterPK   = cipher.MustPubKeyFromHex(ctx.String(MasterPublicKey))
		memoryMode = ctx.Bool(MemoryMode)
		testMode   = ctx.Bool(TestMode)
		testSK     = cipher.MustSecKeyFromHex(ctx.String(TestSecretKey))
		testCount  = ctx.Int(TestInjectionCount)
	)

	var (
		chainDB iko.ChainDB
		stateDB iko.StateDB
	)

	// Prepare ChainDB.
	switch {
	case memoryMode:
		chainDB = iko.NewMemoryChain(10)
	}

	// Prepare StateDB.
	stateDB = iko.NewMemoryState()

	// Prepare blockchain config.
	bcConfig := &iko.BlockChainConfig{
		CreatorPK: masterPK,
		TxAction: func(tx *iko.Transaction) error {
			return nil
		},
	}

	// Prepare blockchain.
	bc, e := iko.NewBlockChain(bcConfig, chainDB, stateDB)
	if e != nil {
		return e
	}
	defer bc.Close()
	log.Info("finished preparing blockchain")

	// Prepare test data.
	if testMode {
		var tx *iko.Transaction
		for i := 0; i < testCount; i++ {
			tx = iko.NewGenTx(tx, iko.KittyID(i), testSK)

			log.WithField("tx", tx.String()).
				Debugf("test:tx_inject(%d)", i)

			if e := bc.InjectTx(tx); e != nil {
				return e
			}
		}
	}

	// Prepare wallet.
	os.MkdirAll("wallet", os.FileMode(0700))
	wallet.SetRootDir("wallet")
	walletManager, e := wallet.NewManager()
	if e != nil {
		return e
	}

	// Prepare http server.
	httpServer, e := http.NewServer(
		&http.ServerConfig{
			Address:   ctx.String(HttpAddress),
			EnableGUI: ctx.BoolT(GUI),
			EnableTLS: false,
		},
		&http.Gateway{
			IKO:    bc,
			Wallet: walletManager,
		},
	)
	if e != nil {
		return e
	}
	defer httpServer.Close()

	<-quit
	return nil
}

func main() {
	if e := app.Run(os.Args); e != nil {
		log.Println(e)
	}
}

// CatchInterrupt catches Ctrl+C behaviour.
func CatchInterrupt() chan int {
	quit := make(chan int)
	go func(q chan<- int) {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt)
		<-sigChan
		signal.Stop(sigChan)
		q <- 1
	}(quit)
	return quit
}
