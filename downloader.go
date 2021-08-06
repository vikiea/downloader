package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/k0kubun/go-ansi"
	"github.com/vikieq/downloader/utils"

	"github.com/schollz/progressbar/v3"
)

type Downloader struct {
	concurrentNum int
	resume        bool
	bar           *progressbar.ProgressBar
	detail        bool
}

func NewDownloader(concurrentNum int, resume, detail bool) *Downloader {
	return &Downloader{concurrentNum: concurrentNum, resume: resume, detail: detail}
}

func (d *Downloader) Download(strUrl, filename string) error {
	if filename == "" {
		filename = path.Base(strUrl)
	}
	resp, err := http.Head(strUrl)
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusOK {
		if resp.Header.Get("Accept-Ranges") == "bytes" {
			return d.multiDownload(strUrl, filename, resp.ContentLength)
		} else {
			return d.singleDownload(strUrl, filename)
		}
	} else {
		log.Fatal(resp.Status)
		return nil
	}
}

func (d *Downloader) multiDownload(url string, filename string, contentLen int64) error {
	log.Printf("多线程下载开启,共%d线程", d.concurrentNum)
	log.Printf("数据总大小:%s", utils.FormatFileSize(contentLen))
	if d.detail {
		d.bar = d.getBar(contentLen, fmt.Sprintf("[cyan][%d/%d][reset]downloading...", d.concurrentNum, d.concurrentNum))
	} else {
		d.bar = d.getBar(contentLen, fmt.Sprintf("downloading..."))
	}
	partSize := contentLen / int64(d.concurrentNum)

	// 创建部分文件的存放目录
	partDir := d.getPartDir(filename)

	defer func(path string) {
		_ = os.RemoveAll(path)
	}(partDir)

	var wg sync.WaitGroup
	wg.Add(d.concurrentNum)

	var rangeStart, rangeEnd int64
	for i := 0; i < d.concurrentNum; i++ {
		rangeEnd = rangeStart + partSize

		go func(i int, rangeStart, rangeEnd int64) {
			defer func() {
				if r := recover(); r != nil {
					log.Fatal("\n啊不好意思,程序崩溃啦,哈哈哈哈哈")
				}
			}()
			defer wg.Done()

			if i == d.concurrentNum-1 {
				rangeEnd = contentLen - 1
			}
			var bar *progressbar.ProgressBar
			if d.detail {
				log.Printf("线程%d,起始长度%#x,结束长度%#x", i, rangeStart, rangeEnd)
				bar = d.getBar(rangeEnd-rangeStart+1, fmt.Sprintf("[cyan][%d/%d][reset]downloading...", i, d.concurrentNum))
			}
			var downloaded int64
			if d.resume {
				partFilename := d.getPartFilename(filename, i)
				content, err := ioutil.ReadFile(partFilename)
				if err == nil {
					downloaded = int64(len(content))
				}
				if d.detail && bar != nil {
					_ = bar.Add64(downloaded)
				}
				_ = d.bar.Add64(downloaded)
			}
			d.downloadPartial(url, filename, rangeStart+downloaded, rangeEnd, i, bar)
		}(i, rangeStart, rangeEnd)

		rangeStart += partSize + 1
	}

	wg.Wait()

	//合并文件
	err := d.mergeFile(filename)
	if err != nil {
		log.Fatal(err)
	}
	_ = d.bar.Finish()
	log.Println("\n下载完成!")
	return nil
}

func (d *Downloader) singleDownload(url string, filename string) error {
	log.Println("单线程下载")
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	d.bar = d.getBar(resp.ContentLength, "downloading...")

	fileDir := path.Dir(filename)
	if !utils.IsExist(fileDir) {
		_ = os.MkdirAll(fileDir, 0777)
	}
	flags := os.O_CREATE | os.O_WRONLY
	destFile, err := os.OpenFile(filename, flags, 0666)
	if err != nil {
		log.Fatal(err)
	}
	defer func(destFile *os.File) {
		_ = destFile.Close()
	}(destFile)

	buf := make([]byte, 32*1024)
	_, err = io.CopyBuffer(io.MultiWriter(destFile, d.bar), resp.Body, buf)
	if err != nil {
		if err == io.EOF {
			return nil
		}
		log.Fatal(err)
	}
	return nil
}

//downloadPartial
//@Description: 分片下载
func (d *Downloader) downloadPartial(url, filename string, rangeStart, rangeEnd int64, i int, bar *progressbar.ProgressBar) {
	if rangeStart > rangeEnd {
		return
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", rangeStart, rangeEnd))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	flags := os.O_CREATE | os.O_WRONLY
	if d.resume {
		flags |= os.O_APPEND
	}

	partFile, err := os.OpenFile(d.getPartFilename(filename, i), flags, 0666)
	if err != nil {
		log.Fatal(err)
	}

	defer func(partFile *os.File) {
		_ = partFile.Close()
	}(partFile)

	buf := make([]byte, 32*1024)
	if bar != nil {
		defer func(bar *progressbar.ProgressBar) {
			_ = bar.Finish()
		}(bar)
		_, err = io.CopyBuffer(io.MultiWriter(partFile, d.bar, bar), resp.Body, buf)
	} else {
		_, err = io.CopyBuffer(io.MultiWriter(partFile, d.bar), resp.Body, buf)
	}
	if err != nil {
		if err == io.EOF {
			return
		}
		log.Fatal(err)
	}
}

// getPartDir 部分文件存放的目录
func (d *Downloader) getPartDir(filename string) string {
	fileDir, basename := filepath.Split(filename)
	partDir := strings.SplitN(basename, ".", 2)[0]
	filePath := filepath.Join(fileDir, partDir)
	if !utils.IsExist(filePath) {
		_ = os.MkdirAll(filePath, 0777)
	}
	return filePath
}

// getPartFilename 构造部分文件的名字
func (d *Downloader) getPartFilename(filename string, partNum int) string {
	_, basename := filepath.Split(filename)
	partDir := d.getPartDir(filename)
	return fmt.Sprintf("%s/%s-%d", partDir, basename, partNum)
}

func (d *Downloader) mergeFile(filename string) error {
	if d.detail {
		log.Println("\n正在整合下载文件...")
	}
	destFile, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatal(err)
	}
	defer func(destFile *os.File) {
		_ = destFile.Close()
	}(destFile)

	for i := 0; i < d.concurrentNum; i++ {
		partFilename := d.getPartFilename(filename, i)
		partFile, _ := os.Open(partFilename)
		_, _ = io.Copy(destFile, partFile)
		_ = partFile.Close()
		_ = os.Remove(partFilename)
	}
	return nil
}

func (d *Downloader) getBar(length int64, desc string) *progressbar.ProgressBar {
	return progressbar.NewOptions64(
		length,
		progressbar.OptionSetWriter(ansi.NewAnsiStdout()),
		progressbar.OptionUseANSICodes(true),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionShowBytes(true),
		progressbar.OptionSetWidth(50),
		progressbar.OptionSetDescription(desc),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[green]=[reset]",
			SaucerHead:    "[green]>[reset]",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}),
	)
}
