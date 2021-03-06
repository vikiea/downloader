package main

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/urfave/cli/v2"
)

func main() {
	//默认并发数
	concurrentNum := runtime.NumCPU()

	app := &cli.App{
		Name:        "多线程文件下载器",
		HelpName:    "downloader",
		Usage:       "通过参数控制,实现并发下载",
		Version:     "v0.0.5",
		Description: "支持断点续传,多线程并发的酷酷的下载器",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "url",
				Aliases:  []string{"u"},
				Usage:    "`URL` to download",
				Required: true,
			},
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Usage:   "Output `filename`",
			},
			&cli.IntFlag{
				Name:    "concurrency",
				Aliases: []string{"n"},
				Value:   concurrentNum,
				Usage:   "Concurrency `number`",
			},
			&cli.BoolFlag{
				Name:    "resume",
				Aliases: []string{"r"},
				Value:   true,
				Usage:   "Resume download",
			},
			&cli.BoolFlag{
				Name:    "detail",
				Aliases: []string{"d"},
				Value:   false,
				Usage:   "Download details",
			},
		},
		Action: func(c *cli.Context) error {
			strURL := c.String("url")
			filename := c.String("output")
			concurrency := c.Int("concurrency")
			resume := c.Bool("resume")
			detail := c.Bool("detail")
			return NewDownloader(concurrency, resume, detail).Download(strURL, filename)
		},
		CommandNotFound: func(*cli.Context, string) { panic("没有这个命令哦") },
		OnUsageError:    func(*cli.Context, error, bool) error { panic("您的用法不对哦") },
		Compiled:        time.Time{},
		Authors:         []*cli.Author{{"vikieq", "flyingqfl@gmail.com"}},
		ExtraInfo:       func() map[string]string { panic("别瞎搞,简单点儿") },
	}
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("err:", r)
			_ = app.Run([]string{"h"})
		}
	}()
	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
