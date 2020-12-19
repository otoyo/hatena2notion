# hatena2notion

はてなブログのエクスポートデータから Notion のインポートデータを作成します。

はてなブログの表現を完全に再現することはできませんが、ある程度体裁の取れた状態で Notion へインポートすることができます。

**Notion の非公式 API を使用しているため使用は自己責任でお願いします。**

## 実行環境

`go env` が異なる場合は Go をインストールして `./hatena2notion` の代わりに `go run main.go` してください。

```sh
GOARCH="amd64"
GOOS="darwin"
```


## 設定

Notion トークンを環境変数 `NOTION_TOKEN` に設定します。

Notion トークンの取得方法は下記の記事を参照してください。

* [トークンの取得方法 / How to get your token](https://www.notion.so/How-to-get-your-token-d7a3421b851f406380fb9ff429cd5d47)

```sh
$ export NOTION_TOKEN=<YourNotionToken>
```

オプションで置換したい URL を指定できます (完全一致のみ)。

```sh
$ export OLD_URL='https://alpacat.hatenablog.com/entry/'
$ export NEW_URL='https://alpacat.com/blog/'
```

## インポートデータの作成

インポートデータの作成は、(1)エクスポートデータの抽出・整形、(2)画像のダウンロードと Notion へのアップロード、画像 URL の置き換えという手順で行います。

### (1) エクスポートデータの抽出・整形

`<export_file>` にはてなブログのエクスポートデータファイルを指定して下記を実行します。

```sh
$ ./hatena2notion -f <export_file> extract
```

整形不可能な Amazon 商品へのリンクを含むファイルが画面に出力されます。インポート後にご確認ください。

`csv/meta.csv` にメタデータファイルを作成します。メタデータファイルは Notion に Table (Database) としてインポート (Merge with CSV) してください。

### (2) 画像のダウンロードと Notion へのアップロード、画像 URL 置き換え

下記コマンドを実行します。記事毎に3秒のスリープを入れているため時間がかかります。

```sh
$ ./hatena2notion upload
```

はてなに保存されている画像 URL が Notion にアップロードした画像 URL に置き換えられ、整形済みの HTML ファイルが `html/` 以下に作成されます。

ページタイトルに `/` を含むものは `:` に置換されています。

HTML ファイルは手動で Notion へ一括インポートしてください。

また、ダウンロードした画像は `images/` 以下に保存されます。

## 機能詳細

* はてなに保存されている画像は非公式 API によって Notion にアップロードされ、画像 URL も置換されます
* cite 形式のリンクは通常の a タグのリンクに置き換えられます
* iframe は可能であれば a タグのリンクに置き換えられます
* 脚注の a タグは削除されます
* Amazon 商品へのリンクは再現できません(手動で対応してください)
* `--` だけの行は `<hr/>` に置換されます
