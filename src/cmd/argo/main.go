package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func main() {
	addr := flag.String("addr", "localhost:4090", "argod 地址")
	flag.Parse()

	client := NewClient(*addr)
	voyageID := selectVoyage(client)
	fmt.Printf("voyage %s 已就绪\n", voyageID)

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}
		prompt := strings.TrimSpace(scanner.Text())
		if prompt == "" || prompt == "/quit" {
			break
		}
		handlePrompt(client, voyageID, prompt, nil)
	}
}

func selectVoyage(client *Client) string {
	infos, err := client.ListVoyages()
	if err != nil || len(infos) == 0 {
		id, err := client.CreateVoyage()
		if err != nil {
			fmt.Fprintf(os.Stderr, "创建 voyage 失败: %v\n", err)
			os.Exit(1)
		}
		return id
	}
	fmt.Println("选择 voyage（序号或 /new 新建）:")
	for i, info := range infos {
		name := info.Name
		if name == "" {
			name = info.ID
		}
		fmt.Printf("  %d) %s  (%s)\n", i+1, name, info.LastActiveAt)
	}
	fmt.Print("> ")
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(0)
	}
	choice := strings.TrimSpace(scanner.Text())
	if choice == "/new" {
		id, err := client.CreateVoyage()
		if err != nil {
			fmt.Fprintf(os.Stderr, "创建 voyage 失败: %v\n", err)
			os.Exit(1)
		}
		return id
	}
	n, err := strconv.Atoi(choice)
	if err != nil || n < 1 || n > len(infos) {
		fmt.Fprintf(os.Stderr, "无效选择\n")
		os.Exit(1)
	}
	return infos[n-1].ID
}

func handlePrompt(client *Client, voyageID, prompt string, decisions map[string]int) {
	ch, err := client.Prompt(voyageID, prompt, decisions)
	if err != nil {
		fmt.Fprintf(os.Stderr, "prompt 失败: %v\n", err)
		return
	}
	for ev := range ch {
		switch ev.Type {
		case "textDelta", "textDone":
			if ev.Delta != "" {
				fmt.Print(ev.Delta)
			}
		case "error":
			fmt.Fprintf(os.Stderr, "\n错误: %s\n", ev.Err)
			return
		case "ask":
			decisions := handleAsk(ev.Omens)
			handlePrompt(client, voyageID, "", decisions)
			return
		case "done":
			fmt.Println()
			return
		}
	}
}

func handleAsk(omens []Omen) map[string]int {
	fmt.Println("\n待审批工具调用:")
	for i, o := range omens {
		fmt.Printf("  %d) %s: %s\n", i+1, o.Name, o.Arguments)
	}
	fmt.Print("[a]pprove / [d]eny / [s]kip: ")
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return nil
	}
	decisions := make(map[string]int)
	choice := strings.TrimSpace(scanner.Text())
	switch choice {
	case "a":
		for _, o := range omens {
			decisions[o.ID] = 1 // VerdictApprove
		}
	case "d":
		for _, o := range omens {
			decisions[o.ID] = 2 // VerdictDeny
		}
	}
	return decisions
}
