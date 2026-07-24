package main

import "embed"

//go:embed all:web/dist
var embedded embed.FS
