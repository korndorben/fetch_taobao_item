package main

import (
	"net/http"
	"log"
	"github.com/PuerkitoBio/goquery"
	"time"
	"strings"
	"github.com/djimenez/iconv-go"
	"os"
	"encoding/json"
	"fmt"
	"flag"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"io/ioutil"
)

var (
	config *Config
	EMPTY  = ""
)

func init() {
	config, _ = LoadConfiguration("./taobao.item.config.json")
}

func main() {
	var addr = flag.String("addr", ":8080", "http service address")
	flag.Parse()

	// 构造路由表
	r := mux.NewRouter()

	//查看宝贝详情
	r.HandleFunc("/{id}.html", func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		var beginTime = time.Now()

		vars := mux.Vars(r)
		if len(vars["id"]) <= 0 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var item, err = ProcessTaobaoItem(vars["id"])
		item.Executed = time.Now().Sub(beginTime).Nanoseconds() / 1e6
		response, err := json.Marshal(item)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// 将结果透传给前端
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(response)))
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(response); err != nil {
			fmt.Println("main:", err.Error())
		}
	})

	http.Handle("/", r)
	if err := http.ListenAndServe(*addr, r); err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

func GetTaobaoItem(id string) (string, error) {
	url := fmt.Sprintf(config.Url, id)
	//默认的请求器
	var netClient = &http.Client{
		Timeout: time.Second * 10,
	}

	req, err := http.NewRequest("GET", url, strings.NewReader(""))
	if err != nil {
		fmt.Printf("请求的时候报错:%v", err)
		return EMPTY, err
	}

	req.Header.Add("Accept-Charset", "utf-8")
	// Request the HTML page.
	res, err := netClient.Do(req)
	if err != nil {
		fmt.Printf("请求的时候报错:%v", err)
		return EMPTY, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		fmt.Printf("status code error: %d %s", res.StatusCode, res.Status)
		return EMPTY, errors.New("请求失败")
	}

	response, err := ioutil.ReadAll(res.Body)
	if err != nil {
		fmt.Printf("请求的时候报错:%v", err)
		return EMPTY, err
	}
	return iconv.ConvertString(string(response), "gb18030", "utf-8")
}

func ProcessTaobaoItem(id string) (*TaobaoItem, error) {
	response, err := GetTaobaoItem(id)
	if err != nil {
		fmt.Printf("读取源数据时报错:%v", err)
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(response))
	fmt.Printf("doc:%v", doc)
	if err != nil {
		fmt.Printf("读取源数据时报错:%v", err)
		return nil, err
	}
	var item = &TaobaoItem{}

	if ftitle := doc.Find(config.Rules.Title); ftitle != nil {
		if title, exists := ftitle.Attr("data-title"); exists {
			log.Printf("标题\t%s", title)
			item.Title = title
		}
	}

	if fprice := doc.Find(config.Rules.Price); fprice != nil {
		log.Printf("价格\t%s", fprice.Text())
		item.Price = fprice.Text()
	}

	log.Printf(">销售属性")
	item.SaleProps = make([]Prop, 0)
	if fsaleprop := doc.Find(config.Rules.SaleProps); fsaleprop != nil {
		fsaleprop.Each(func(index int, saleprop *goquery.Selection) { //三个，尺码、颜色、结构
			if props := saleprop.Find("li"); props != nil {

				//属性标题
				if text, exists := saleprop.Attr("data-property"); exists {
					log.Printf("\t%s---------", text)

					//属性列表
					if fsize := saleprop.Find("li"); fsize != nil { //属性列表
						fmt.Printf("节点长度:%d\n", fsize.Length())

						fsize.Each(func(index int, el *goquery.Selection) {
							if text, exists := el.Attr("data-value"); exists {
								prop := Prop{}

								//code
								prop.Code = text
								log.Printf("\t%s", text)

								//image
								if detail := el.Find("a"); detail != nil {
									if style, exists := detail.Attr("style"); exists {
										log.Printf("\t%s", style[15:len(style)-19])
										prop.Image = style[15 : len(style)-19]
									}
								}

								//value
								if span := el.Find("span"); span != nil {
									log.Printf("\t%s", span.Text())
									prop.Value = span.Text()
								}
								item.SaleProps = append(item.SaleProps, prop)
							}
						})
					}
					log.Print("\n")
				}
			}
		})
	}

	log.Printf(">非销售属性")

	//属性列表
	item.NonSaleProps = make([]Prop, 0)
	if fnonsaleprop := doc.Find(config.Rules.NonSaleProps); fnonsaleprop != nil {
		fnonsaleprop.Each(func(index int, el *goquery.Selection) {
			//text, _ := item.Attr("title")
			log.Printf("\t%s", el.Text())
			var kvpare = strings.Split(el.Text(), ":")
			item.NonSaleProps = append(item.NonSaleProps, Prop{
				Code:  strings.TrimSpace(kvpare[0]),
				Value: strings.TrimSpace(kvpare[1]),
			})
		})
	}

	return item, nil
}

type TaobaoItem struct {
	Title        string `json:"title,omitempty"`
	Price        string `json:"price,omitempty"`
	SaleProps    []Prop `json:"sale_props,omitempty"`
	NonSaleProps []Prop `json:"non_sale_props,omitempty"`
	Executed     int64  `json:"executed"`
}

type Prop struct {
	Code  string `json:"code,omitempty"`
	Value string `json:"value,omitempty"`
	Image string `json:"image,omitempty"`
}

func LoadConfiguration(filename string) (*Config, error) {
	var config Config
	configFile, err := os.Open(filename)
	defer configFile.Close()
	if err != nil {
		log.Println(err)
		return nil, err
	}
	jsonParser := json.NewDecoder(configFile)
	jsonParser.Decode(&config)
	return &config, nil
}

//淘宝抓取规则配置表
type Config struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Url     string `json:"url"`
	Rules struct {
		Title        string `json:"title"`
		Price        string `json:"price"`
		SaleProps    string `json:"sale-props"`
		NonSaleProps string `json:"non-sale-props"`
	} `json:"rules"`
}
