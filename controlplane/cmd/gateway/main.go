package main

import "strix/internal/gateway/httpapi"

func main() {
	gw, closer, err := httpapi.NewGateway()
	if err != nil {
		panic(err.Error())
	}
	defer closer()
	if err = gw.Run(); err != nil {
		panic(err.Error())
	}
}
