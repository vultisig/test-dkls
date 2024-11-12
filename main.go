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
				Required:   true,
				HasBeenSet: false,
				Hidden:     false,
			},
			&cli.StringSliceFlag{
				Name:       "parties",
				Aliases:    []string{"p"},
				Usage:      "comma separated list of party keys, need to have all the keys of the keygen committee",
				Required:   true,
				HasBeenSet: false,
				Hidden:     false,
			},
			&cli.StringFlag{
				Name:       "session",
				Usage:      "current communication session",
				Required:   true,
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
				},
				Action: keygenCmd,
			},
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
	tss, err := NewTssService(server)
	if err != nil {
		return err
	}
	return tss.Keygen(sessionID, chaincode, key, parties, isLeader)
}
