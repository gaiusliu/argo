package deck

// Deck 工具注册表，每请求构造，非全局单例。
type Deck struct {
	hands map[string]Hand
	clip  Clip
}

// NewDeck 构造 Deck，builtin 工具自动包裹 Clip 后处理。
func NewDeck(builtin []Hand, clip Clip) *Deck {
	d := &Deck{hands: make(map[string]Hand), clip: clip}
	for _, t := range builtin {
		d.hands[t.Name()] = &truncatedTool{Hand: t, clip: d.clip}
	}
	return d
}

// List 返回全部工具的 Hand 描述符，供 sight 构造 API 请求。
func (d *Deck) List() []Hand {
	result := make([]Hand, 0, len(d.hands))
	for _, t := range d.hands {
		result = append(result, t)
	}
	return result
}

// Lookup 按名称查找工具。
func (d *Deck) Lookup(name string) (Hand, bool) {
	t, ok := d.hands[name]
	return t, ok
}
