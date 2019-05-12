#!/usr/bin/env bash
echo '获取包...'
go get -u github.com/gorilla/mux
go get -u github.com/PuerkitoBio/goquery
go get -u github.com/djimenez/iconv-go
echo '...获取完毕'
echo '正在编译...' && go build && echo '...编译成功' && pwd && ls -lh && ./app