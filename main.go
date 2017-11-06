package main

import (
	"github.com/urfave/cli"
	"os"
	"log"
)

func main() {
	app := cli.NewApp()

	app.Name = "Auto Slice of Life"
	app.Usage = "Generates Slice of Life pictures from security cameras"
	app.Version = "0.0.1"

	app.Commands = []cli.Command{
		{
			Name:   "fetch",
			Usage:  "Fetches an image from a URL at the given interval",
			Action: FetchAction,
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "host",
					Usage: "The host to fetch an image from",
				},
				cli.StringFlag{
					Name:  "user",
					Usage: "The username to provide to the UBNT camera",
					Value: "ubnt",
					EnvVar: "CAMERA_USER",
				},
				cli.StringFlag{
					Name:  "pass",
					Usage: "The password to access the camera",
					EnvVar: "CAMERA_PASS",
				},
				cli.IntFlag{
					Name:  "interval",
					Usage: "The number of seconds between image captures",
				},
				cli.StringFlag{
					Name:  "start",
					Usage: "A time string to start capturing at in 24 hour time, e.g. 13:00:00",
				},
				cli.StringFlag{
					Name:  "prefix",
					Usage: "a folder to place all the images in",
				},
			},
		},
		{
			Name:   "merge",
			Usage:  "Merges a directory of images into a single slice of life image",
			Action: MergeAction,
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "dir",
					Usage: "The directory to look for images in",
					Value: ".",
				},
				cli.BoolTFlag{
					Name: "vertical",
					Usage: "Compose the slice of life image with vertical slices from each image",
				},
				cli.BoolFlag{
					Name: "horizontal",
					Usage: "Compose the slice of life image with horizontal slices from each image",
				},
			},
		},
	}

	err := app.Run(os.Args)
	if nil != err {
		log.Fatal(err.Error())
	}
}
