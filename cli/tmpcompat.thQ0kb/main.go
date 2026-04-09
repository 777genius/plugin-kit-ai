package main

import (
  "encoding/json"
  "fmt"
  "os"

  "github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
)

func main() {
  report, warnings, err := pluginmanifest.InspectSourceRef("github:gastownhall/beads@e54bc625fd7f79624b15ef69561a32a18f44ec3f//claude-plugin", "", "all", false)
  if err != nil {
    fmt.Fprintln(os.Stderr, err)
    os.Exit(1)
  }
  body, err := json.MarshalIndent(struct {
    Warnings []pluginmanifest.Warning `json:"warnings"`
    Report   pluginmanifest.SourceInspection `json:"report"`
  }{Warnings: warnings, Report: report}, "", "  ")
  if err != nil {
    fmt.Fprintln(os.Stderr, err)
    os.Exit(1)
  }
  fmt.Println(string(body))
}
