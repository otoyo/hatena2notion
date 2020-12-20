package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/cheggaaa/pb/v3"
	"github.com/kjk/notionapi"
	"github.com/otoyo/movabletype"
	"golang.org/x/net/html"
)

type Page struct {
	Title   string
	Slug    string
	Status  string
	Body    string
	Date    string
	Tags    string
	Excerpt string
	Author  string
}

const dateFormat = "2006/01/02"

func main() {
	client := &notionapi.Client{}
	client.AuthToken = os.Getenv("NOTION_TOKEN")

	exportFilepath := flag.String("f", "", "Exported File")
	flag.Parse()
	subcommand := flag.Arg(0)

	if subcommand == "extract" {
		extract(*exportFilepath)
		reform()
	} else if subcommand == "upload" {
		upload(client)
	}
}

// MT形式のエクスポートデータからメタデータCSVと記事データHTMLを抽出する
func extract(exportFilepath string) {
	exportFile, err := os.Open(exportFilepath)
	if err != nil {
		log.Fatal(err)
	}
	defer exportFile.Close()

	entries, err := movabletype.Parse(exportFile)
	if err != nil {
		log.Fatal(err)
	}

	metaFile, err := os.Create("csv/meta.csv")
	if err != nil {
		log.Fatal(err)
	}
	defer metaFile.Close()

	writer := csv.NewWriter(metaFile)

	err = writer.Write([]string{"Title", "Slug", "Status", "Date", "Author", "Tags", "Excerpt"})
	if err != nil {
		log.Fatal(err)
	}

	for _, entry := range entries {
		page := &Page{
			Title:   entry.Title,
			Slug:    strings.ReplaceAll(entry.Basename, "/", ""),
			Status:  entry.Status,
			Body:    entry.Body + entry.ExtendedBody,
			Date:    entry.Date.Format(dateFormat),
			Tags:    strings.Join(entry.Category, ", "),
			Excerpt: entry.Excerpt,
			Author:  entry.Author,
		}

		row := []string{page.Title, page.Slug, page.Status, page.Date, page.Author, page.Tags, page.Excerpt}

		err = writer.Write(row)
		if err != nil {
			log.Fatal(err)
		}

		htmlFile, err := os.Create("tmp/" + strings.ReplaceAll(page.Title, "/", ":") + ".html")
		if err != nil {
			log.Fatal(err)
		}

		bytes := []byte(page.Body)
		_, err = htmlFile.Write(bytes)
		if err != nil {
			log.Fatal(err)
		}
		htmlFile.Close()
	}

	writer.Flush()
	err = writer.Error()
	if err != nil {
		log.Fatal(err)
	}
}

func reform() {
	filenames, err := filepath.Glob("tmp/*.html")
	if err != nil {
		log.Fatal(err)
	}

	for _, filename := range filenames {
		file, err := os.Open(filename)
		if err != nil {
			log.Fatal(err)
		}

		doc, err := html.Parse(file)
		if err != nil {
			log.Fatal(err)
		}
		file.Close()

		reformNode(doc, filename)

		file, err = os.Create(filename)
		if err != nil {
			log.Fatal(err)
		}

		err = html.Render(file, doc)
		if err != nil {
			log.Fatal(err)
		}
		file.Close()
	}
}

// ノードを走査して再構築する
func reformNode(node *html.Node, filename string) {
	if node.Type == html.ElementNode {
		replaceFootnote(node)
		replaceIframe(node)
		replaceHr(node)
		replaceLink(node)
		notifyAmazonImgLink(node, filename)
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		reformNode(child, filename)
	}
}

// Amazon 商品への画像リンクがあれば知らせる
func notifyAmazonImgLink(node *html.Node, filename string) {
	if node.Data == "a" {
		amazonLink := regexp.MustCompile(`^https://www\.amazon\.co\.jp`)

		for _, attr := range node.Attr {
			if attr.Key == "href" && amazonLink.MatchString(attr.Val) {
				amazonImgLink := regexp.MustCompile(`amazon-adsystem.com`)

				for child := node.FirstChild; child != nil; child = child.NextSibling {
					if child.Type == html.ElementNode && child.Data == "img" {
						for _, imgAttr := range child.Attr {
							if imgAttr.Key == "src" && amazonImgLink.MatchString(imgAttr.Val) {
								fmt.Printf("確認が必要なAmazonリンクが含まれています: %s\n", filename)
								return
							}
						}
					}
				}
			}
		}
	}
}

// iframe を置き換える
// 1. cite > a を兄弟に持つ iframe の場合、a タグの内容がドメインだけになっている
//   iframe[title] or a[href] をタイトルとして a タグの内容にセットし iframe, cite を削除する
// 2. iframe のみ場合
//   iframe[src] からURLを取得し a タグの href と内容にセットし iframe を削除する
func replaceIframe(node *html.Node) {
	var iframe *html.Node
	var cite *html.Node

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && child.Data == "iframe" {
			iframe = child
		}
		if iframe != nil && child.Type == html.ElementNode && child.Data == "cite" {
			cite = child
		}
	}

	if iframe != nil && cite != nil {
		title := ""
		for _, attr := range iframe.Attr {
			if attr.Key == "title" {
				title = attr.Val
				break
			}
		}

		var href string
		for grandchild := cite.FirstChild; grandchild != nil; grandchild = grandchild.NextSibling {
			if grandchild.Type == html.ElementNode && grandchild.Data == "a" {
				for _, attr := range grandchild.Attr {
					if attr.Key == "href" {
						href = attr.Val
						break
					}
				}
			}
		}

		attrs := make([]html.Attribute, 2)
		attrs[0] = html.Attribute{
			Key: "href",
			Val: href,
		}
		attrs[1] = html.Attribute{
			Key: "target",
			Val: "_blank",
		}
		aNode := &html.Node{
			Type: html.ElementNode,
			Data: "a",
			Attr: attrs,
		}

		if title == "" {
			title = href
		}
		textNode := &html.Node{
			Type: html.TextNode,
			Data: title,
		}
		aNode.AppendChild(textNode)

		node.InsertBefore(aNode, iframe)
		node.RemoveChild(iframe)
		node.RemoveChild(cite)
	} else if iframe != nil {
		for _, attr := range iframe.Attr {
			if attr.Key == "src" {
				attrs := make([]html.Attribute, 2)
				attrs[0] = html.Attribute{
					Key: "href",
					Val: attr.Val,
				}
				attrs[1] = html.Attribute{
					Key: "target",
					Val: "_blank",
				}
				aNode := &html.Node{
					Type: html.ElementNode,
					Data: "a",
					Attr: attrs,
				}

				textNode := &html.Node{
					Type: html.TextNode,
					Data: attr.Val,
				}
				aNode.AppendChild(textNode)

				pNode := &html.Node{
					Type: html.ElementNode,
					Data: "p",
				}
				pNode.AppendChild(aNode)

				node.InsertBefore(pNode, iframe)
				node.RemoveChild(iframe)
				break
			}
		}
	}
}

// -- を <hr/> に置換する
func replaceHr(node *html.Node) {
	hrPattern := regexp.MustCompile(`^--+`)

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && child.Data == "p" {
			for grandchild := child.FirstChild; grandchild != nil; grandchild = grandchild.NextSibling {
				if grandchild.Type == html.TextNode && hrPattern.MatchString(grandchild.Data) {
					hrNode := &html.Node{
						Type: html.ElementNode,
						Data: "hr",
					}

					node.InsertBefore(hrNode, child)
					node.RemoveChild(child)
					break
				}
			}
		}
	}
}

func replaceLink(node *html.Node) {
	oldURL := os.Getenv("OLD_URL")
	newURL := os.Getenv("NEW_URL")

	if oldURL != "" && newURL != "" {
		if node.Type == html.ElementNode && node.Data == "a" {
			for i, attr := range node.Attr {
				if attr.Key == "href" {
					attr.Val = strings.ReplaceAll(attr.Val, oldURL, newURL)
					node.Attr[i] = attr
					break
				}
			}
		}
	}
}

// 脚注リンクの a タグを削除する
func replaceFootnote(node *html.Node) {
	footnoteLink := regexp.MustCompile(`^#`)

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && child.Data == "a" {
			for _, attr := range child.Attr {
				if attr.Key == "href" && footnoteLink.MatchString(attr.Val) {
					textNode := &html.Node{
						Type: html.TextNode,
						Data: child.FirstChild.Data,
					}
					node.InsertBefore(textNode, child)
					node.RemoveChild(child)
					break
				}
			}
		}
	}
}

func upload(client *notionapi.Client) {
	filenames, err := filepath.Glob("tmp/*.html")
	if err != nil {
		log.Fatal(err)
	}

	bar := pb.StartNew(len(filenames))

	for _, filename := range filenames {
		bar.Increment()
		time.Sleep(3 * time.Second)

		file, err := os.Open(filename)
		if err != nil {
			log.Fatal(err)
		}

		doc, err := html.Parse(file)
		if err != nil {
			log.Fatal(err)
		}
		file.Close()

		searchAndReplaceImgURL(client, doc)

		file, err = os.Create("html/" + filepath.Base(filename))
		if err != nil {
			log.Fatal(err)
		}

		err = html.Render(file, doc)
		if err != nil {
			log.Fatal(err)
		}
		file.Close()
	}

	bar.Finish()
}

func searchAndReplaceImgURL(client *notionapi.Client, node *html.Node) {
	if node.Type == html.ElementNode && node.Data == "img" {
		hatenaDomain := regexp.MustCompile(`hatena\.com`)

		for i, attr := range node.Attr {
			if attr.Key == "src" && hatenaDomain.MatchString(attr.Val) {
				filename, err := downloadImageFromURL(attr.Val)
				if err != nil {
					fmt.Printf("Failed to download image. file: %s error: %#v", filename, err)
					continue
				}

				fileURL, err := uploadImageToNotion(client, filename)
				if err != nil {
					fmt.Printf("Failed to upload image. file: %s error: %#v", filename, err)
					continue
				}

				attr.Val = fileURL
				node.Attr[i] = attr
				break
			}
		}
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		searchAndReplaceImgURL(client, child)
	}
}

func downloadImageFromURL(url string) (filename string, err error) {
	filename = filepath.Base(url)

	response, err := http.Get(url)
	if err != nil {
		return filename, err
	}
	defer response.Body.Close()

	file, err := os.Create(filename)
	if err != nil {
		return filename, err
	}
	defer file.Close()

	io.Copy(file, response.Body)

	return filename, nil
}

func uploadImageToNotion(client *notionapi.Client, filename string) (fileURL string, err error) {
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()

	_, fileURL, err = client.UploadFile(file)
	if err != nil {
		return "", err
	}

	err = os.Rename(filename, "images/"+filename)
	if err != nil {
		return "", err
	}

	return fileURL, nil
}
