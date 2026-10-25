package client_test

import (
	"context"
	"fmt"
	"net/http/httptest"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/client"
	"github.com/ricardogayer/go-agui/pkg/server"
)

func Example() {
	srv := httptest.NewServer(server.NewAgentHandler(&server.EchoRunner{}))
	defer srv.Close()

	agent := client.New(srv.URL)
	agent.AddMessage(&agui.UserMessage{ID: "u1", Content: agui.TextContent("Olá")})
	res, err := agent.Run(context.Background(), client.RunInput{}, nil)
	if err != nil {
		panic(err)
	}
	reply := agent.Store.Messages[1].(*agui.AssistantMessage)
	fmt.Println(res.Status, agui.Deref(reply.Content))
	// Output: succeeded Echo: Olá
}
