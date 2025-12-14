package main

import (
	"banyan/addon/sdk"
	"net/http"
)

func main() {
	client := sdk.New()

	_ = client.Disclose("testaddon", "0.1.0", map[string]any{
		"description": "Test addon for development",
	})

	// Optional: add alias resolver(s)
	client.AddAliasResolver(func(alias string) (string, error) {
		if alias == "example" {
			return "/var/data/example.fig", nil
		}
		return "", nil
	})

	mux := sdk.NewMux().WithLogger(nil)

	mux.GET("/addons/echo", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sdk.WriteText(w, 200, "testaddon local ok")
	}))

	mux.GET("/addons/echo/more", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sdk.WriteText(w, 200, "testaddon extra path ok")
	}))

	mux.GET("/addons/echo/{slug}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slug := sdk.Param(r, "slug")
		sdk.WriteText(w, 200, "testaddon slug="+slug)
	}))

	mux.RGET("/addons/echo-remote", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sdk.WriteText(w, 200, "testaddon remote ok")
	}))

	_ = client.RunMux(mux)
}
