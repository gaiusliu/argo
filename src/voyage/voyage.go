package voyage

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"argo/src/board"
	"argo/src/deck"
	"argo/src/lore"
	"argo/src/pact"
	"argo/src/press"
	"argo/src/sight"
	"argo/src/vault"
)

// Voyage 一次航程，持有所有域模块实例，自驱动执行六态循环。
type Voyage struct {
	Info         VoyageInfo
	phase        int
	sight        sight.Sight
	board        *board.Board
	journal      vault.Journal
	deck         *deck.Deck
	lore         *lore.Lore
	omens        []pact.Omen
	press        press.Press
	tokenUsage   pact.TokenUsage
	msgs         []pact.Message
	journalIndex int
	ctx          context.Context
	out          chan<- pact.Event
}

// New 装配一次航程，从 voyages/{voyageID}/ 加载配置。
func New(ctx context.Context, cfg AgentCfg, voyageID string) (*Voyage, error) {
	info, err := loadInfo(voyageID)
	if err != nil {
		return nil, err
	}
	client := &http.Client{}
	d := deck.New(deck.BuiltinTools(), deck.NewClip(cfg.Clip))
	hands := make([]deck.HandMeta, 0, len(d.List()))
	for _, h := range d.List() {
		hands = append(hands, deck.HandMeta{
			Name:        h.Name(),
			Description: h.Description(),
			Parameters:  h.ParametersSchema(),
		})
	}
	s, err := sight.New(cfg.Sight, cfg.Sight.DefaultModel, hands, client)
	if err != nil {
		return nil, err
	}
	p, err := press.New(cfg.Press, s)
	if err != nil {
		return nil, err
	}
	l := lore.New(argoHome()+"/skills", ".argo/skills")
	b := board.New(cfg.Board.Iron, cfg.Board.Cord)
	return &Voyage{
		Info:    info,
		phase:   PhaseSight,
		ctx:     ctx,
		sight:   s,
		board:   b,
		journal: vault.NewFileJournal(voyageDir(voyageID) + "/journal.jsonl"),
		deck:    d,
		lore:    l,
		press:   p,
	}, nil
}

// AppendUserMessage 追加用户消息到对话历史。
func (v *Voyage) AppendUserMessage(prompt string) {
	v.msgs = append(v.msgs, pact.Message{
		Role:      pact.RoleUser,
		Content:   prompt,
		Timestamp: time.Now().Format(time.RFC3339),
	})
}

// Messages 返回当前对话历史。
func (v *Voyage) Messages() []pact.Message {
	return v.msgs
}

// Resume 恢复待审批会话，从最后一条 assistant 消息恢复 omens 并更新 verdict。
// decisions 的 key 为 Omen.ID，value 取值见 pact.VerdictApprove / pact.VerdictDeny。
func (v *Voyage) Resume(decisions map[string]int) error {
	if len(v.msgs) == 0 {
		return fmt.Errorf("resume: no message history")
	}
	last := v.msgs[len(v.msgs)-1]
	if last.Role != pact.RoleAssistant || len(last.Omens) == 0 {
		return fmt.Errorf("resume: last message is not assistant with omens")
	}
	v.omens = last.Omens
	for i := range v.omens {
		o := &v.omens[i]
		verdict, ok := decisions[o.ID]
		if !ok {
			continue
		}
		o.Verdict = verdict
		if verdict == pact.VerdictApprove {
			v.board.Pass(*o)
		}
	}
	v.phase = PhaseSteer
	return nil
}

// CancelAsking 取消待审批状态：将 journal 最后一条 assistant 消息的 VerdictAsk 改为 VerdictDeny。
func (v *Voyage) CancelAsking() error {
	lines, err := v.journal.Scan()
	if err != nil {
		return err
	}
	if len(lines) == 0 {
		return fmt.Errorf("cancel asking: empty journal")
	}
	last := lines[len(lines)-1]
	var msg pact.Message
	if err := json.Unmarshal([]byte(last), &msg); err != nil {
		return err
	}
	if msg.Role != pact.RoleAssistant {
		return fmt.Errorf("cancel asking: last message is not assistant")
	}
	changed := false
	for i := range msg.Omens {
		if msg.Omens[i].Verdict == pact.VerdictAsk {
			msg.Omens[i].Verdict = pact.VerdictDeny
			changed = true
		}
	}
	if !changed {
		return nil
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	lines[len(lines)-1] = string(data)
	path := voyageDir(v.Info.ID) + "/journal.jsonl"
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644)
}
// SaveInfo 更新 LastActiveAt 到 voyage.json。
func (v *Voyage) SaveInfo() error {
	v.Info.LastActiveAt = time.Now()
	return saveInfo(v.Info)
}

// LoadMessages 从 journal 加载历史消息并赋值给 v.msgs。
func (v *Voyage) LoadMessages() error {
	lines, err := v.journal.Scan()
	if err != nil {
		return err
	}
	var msgs []pact.Message
	for _, line := range lines {
		var m pact.Message
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			continue
		}
		msgs = append(msgs, m)
	}
	v.msgs = msgs
	return nil
}

// SetSail 扬帆起航，启动六态自驱动循环，返回流式事件 channel。
func (v *Voyage) SetSail() <-chan pact.Event {
	out := make(chan pact.Event, 8)
	v.out = out
	go func() {
		defer close(out)
		for {
			switch v.phase {
			case PhaseSight:
				v.runSight()
			case PhaseRead:
				v.runRead()
			case PhaseSteer:
				v.runSteer()
			case PhaseCall:
				v.runCall()
				return
			case PhaseHeave:
				v.runHeave()
			case PhaseDock:
				v.runDock()
				return
			}
		}
	}()
	return out
}
