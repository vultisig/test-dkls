package main

import (
	"os"

	"github.com/urfave/cli/v2"
)

func main() {
	app := cli.App{
		Name:  "dkls-test",
		Usage: "A tool to test dkls keygen & keysign",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "server",
				Aliases: []string{"s"},
				Usage:   "server address",
				Value:   "http://127.0.0.1:9090",
			},
			&cli.StringFlag{
				Name:       "key",
				Aliases:    []string{"k"},
				Usage:      "something to uniquely identify local party",
				Required:   false,
				HasBeenSet: false,
				Hidden:     false,
			},
			&cli.StringSliceFlag{
				Name:       "parties",
				Aliases:    []string{"p"},
				Usage:      "comma separated list of party keys, need to have all the keys of the keygen committee",
				Required:   false,
				HasBeenSet: false,
				Hidden:     false,
			},
			&cli.StringFlag{
				Name:       "session",
				Usage:      "current communication session",
				Required:   false,
				HasBeenSet: false,
				Hidden:     false,
			},
			&cli.BoolFlag{
				Name:       "leader",
				Usage:      "leader will make sure all parties present , and kick off the process(keygen/reshare/keysign)",
				Required:   false,
				Hidden:     false,
				HasBeenSet: false,
				Value:      false,
			},
		},
		Commands: []*cli.Command{
			{
				Name: "keygen",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:       "chaincode",
						Aliases:    []string{"cc"},
						Usage:      "hex encoded chain code",
						Required:   true,
						HasBeenSet: false,
						Hidden:     false,
					},
					&cli.BoolFlag{
						Name:       "eddsa",
						Required:   false,
						Hidden:     false,
						HasBeenSet: false,
						Value:      false,
					},
				},
				Action: keygenCmd,
			},
			{
				Name: "export",
				Flags: []cli.Flag{
					&cli.StringSliceFlag{
						Name:       "part",
						Aliases:    []string{"p"},
						Usage:      "part files",
						Required:   true,
						HasBeenSet: false,
						Hidden:     false,
					},
				},
				Action: exportCmd,
			},
			{
				Name: "reshare",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:       "pubkey",
						Aliases:    []string{"pk"},
						Usage:      "ECDSA pubkey that will be used to do keysign",
						Required:   true,
						HasBeenSet: false,
						Hidden:     false,
					},
					&cli.StringSliceFlag{
						Name:       "old-parties",
						Usage:      "Old parties",
						Required:   true,
						Hidden:     false,
						HasBeenSet: false,
					},
				},
				Action: reshareCmd,
			},
			{
				Name: "keysign",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:       "pubkey",
						Aliases:    []string{"pk"},
						Usage:      "ECDSA pubkey that will be used to do keysign",
						Required:   true,
						HasBeenSet: false,
						Hidden:     false,
					},
					&cli.StringFlag{
						Name:       "message",
						Aliases:    []string{"m"},
						Usage:      "message that need to be signed",
						Required:   true,
						HasBeenSet: false,
						Hidden:     false,
					},
					&cli.StringFlag{
						Name:     "derivepath",
						Usage:    "derive path for bitcoin, e.g. m/84'/0'/0'/0/0",
						Required: true,
					},
					&cli.BoolFlag{
						Name:       "eddsa",
						Required:   false,
						Hidden:     false,
						HasBeenSet: false,
						Value:      false,
					},
				},
				Action: keysignCmd,
			},
		},
		Before: func(c *cli.Context) error {
			if c.Command.Name == "export" {
				return nil
			}
			return nil
		},
	}

	if err := app.Run(os.Args); err != nil {
		panic(err)
	}
}

func keygenCmd(c *cli.Context) error {
	key := c.String("key")
	parties := c.StringSlice("parties")
	sessionID := c.String("session")
	server := c.String("server")
	chaincode := c.String("chaincode")
	isLeader := c.Bool("leader")
	localStateAccessorImp := NewLocalStateAccessorImp(key)
	isEdDSA := c.Bool("eddsa")
	tss, err := NewTssService(server, localStateAccessorImp, isEdDSA)
	if err != nil {
		return err
	}
	return tss.Keygen(sessionID, chaincode, key, parties, isLeader)
}

// reshare doesn't work yet
func reshareCmd(c *cli.Context) error {
	key := c.String("key")
	parties := c.StringSlice("parties")
	sessionID := c.String("session")
	server := c.String("server")
	publicKey := c.String("pubkey")
	isLeader := c.Bool("leader")
	isEdDSA := c.Bool("eddsa")
	oldParties := c.StringSlice("old-parties")
	localStateAccessorImp := NewLocalStateAccessorImp(key)
	tss, err := NewTssService(server, localStateAccessorImp, isEdDSA)
	if err != nil {
		return err
	}
	return tss.Reshare(sessionID, publicKey, key, parties, oldParties, isLeader)
}
func keysignCmd(c *cli.Context) error {
	key := c.String("key")
	parties := c.StringSlice("parties")
	sessionID := c.String("session")
	server := c.String("server")
	isLeader := c.Bool("leader")
	publicKey := c.String("pubkey")
	message := c.String("message")
	derivePath := c.String("derivepath")
	isEdDSA := c.Bool("eddsa")
	localStateAccessorImp := NewLocalStateAccessorImp(key)
	tss, err := NewTssService(server, localStateAccessorImp, isEdDSA)
	if err != nil {
		return err
	}
	return tss.Keysign(sessionID, publicKey, message, derivePath, key, parties, isLeader)
}
func exportCmd(c *cli.Context) error {
	parts := c.StringSlice("part")
	parties := c.StringSlice("parties")

	return ExportRootKey(parts, parties)
}
