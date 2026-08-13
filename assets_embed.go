package main

import "embed"

//go:embed all:web/dist
//go:embed web/dist/index.html
var embedded embed.FS
