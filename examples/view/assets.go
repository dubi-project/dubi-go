package main

import (
	"embed"

	dubi "github.com/dubi-project/dubi-go"
)

//go:embed asset
var assets embed.FS

func init() { dubi.AssetFS(assets) }
