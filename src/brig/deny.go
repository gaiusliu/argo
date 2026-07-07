package brig

// loadStaticRules 合并全部静态规则组，返回独立副本供 Engine 加载。
//
// 规则组加载顺序即匹配优先级：先加载的规则组先被 resolveStaticRules 遍历。
// 因此 denyRules 必须排在第一位——确保 Deny 规则优先于 Allow/Ask 命中，
// 否则 "rm -rf /" 可能被 dangerCommands 的 "rm" Ask 规则先拦截，绕过 Deny。
// 新增规则组时，破坏性操作（Deny）必须放在安全命令（Allow）之前。
func loadStaticRules() []Rule {
	var all []Rule

	// 1. deny 库 — 不可逆的破坏性操作，命中即 Deny
	all = append(all, denyRules...)

	// 2. 安全命令 — 纯读取/查询类命令，直接 Allow
	all = append(all, safeCommands...)

	// 3. 危险命令 — 可能修改系统/文件状态，需用户确认
	all = append(all, dangerCommands...)

	// 4. 脚本解释器 — 无法预知脚本行为，需用户确认
	all = append(all, interpreterCommands...)

	// 5. 敏感文件 — 读取/搜索操作，防止泄露
	all = append(all, sensitiveFiles...)

	return all
}

// denyRules 不可绕过的静态拒绝规则
var denyRules = []Rule{
	// 破坏性命令
	{Action: Deny, Tool: "bash", Pattern: "rm -rf /"},
	{Action: Deny, Tool: "bash", Pattern: "rm -rf /*"},
	{Action: Deny, Tool: "bash", Pattern: "mkfs"},
	{Action: Deny, Tool: "bash", Pattern: "dd if="},
	{Action: Deny, Tool: "bash", Pattern: ":(){ :|:& };:"},

	// SSRF 防护 — 禁止访问内网地址
	{Action: Deny, Tool: "web_fetch", Pattern: "127.0.0.1"},
	{Action: Deny, Tool: "web_fetch", Pattern: "localhost"},
	{Action: Deny, Tool: "web_fetch", Pattern: "0.0.0.0"},

	// 系统关键文件 — 拒绝写入
	{Action: Deny, Tool: "write", Pattern: "/etc/shadow"},
	{Action: Deny, Tool: "edit", Pattern: "/etc/shadow"},
	{Action: Deny, Tool: "write", Pattern: "C:\\Windows\\System32"},
	{Action: Deny, Tool: "edit", Pattern: "C:\\Windows\\System32"},
}

// safeCommands 纯读取/查询类 bash 命令，直接 Allow
var safeCommands = []Rule{
	// 文件查看
	{Action: Allow, Tool: "bash", Pattern: "ls"},
	{Action: Allow, Tool: "bash", Pattern: "cat"},
	{Action: Allow, Tool: "bash", Pattern: "head"},
	{Action: Allow, Tool: "bash", Pattern: "tail"},
	{Action: Allow, Tool: "bash", Pattern: "find"},
	{Action: Allow, Tool: "bash", Pattern: "wc"},
	{Action: Allow, Tool: "bash", Pattern: "du"},
	{Action: Allow, Tool: "bash", Pattern: "df"},
	{Action: Allow, Tool: "bash", Pattern: "stat"},
	{Action: Allow, Tool: "bash", Pattern: "file"},
	// 系统信息
	{Action: Allow, Tool: "bash", Pattern: "pwd"},
	{Action: Allow, Tool: "bash", Pattern: "echo"},
	{Action: Allow, Tool: "bash", Pattern: "printf"},
	{Action: Allow, Tool: "bash", Pattern: "sort"},
	{Action: Allow, Tool: "bash", Pattern: "uniq"},
	{Action: Allow, Tool: "bash", Pattern: "cut"},
	{Action: Allow, Tool: "bash", Pattern: "tr"},
	{Action: Allow, Tool: "bash", Pattern: "awk"},
	{Action: Allow, Tool: "bash", Pattern: "which"},
	{Action: Allow, Tool: "bash", Pattern: "whereis"},
	{Action: Allow, Tool: "bash", Pattern: "type"},
	{Action: Allow, Tool: "bash", Pattern: "whoami"},
	{Action: Allow, Tool: "bash", Pattern: "hostname"},
	{Action: Allow, Tool: "bash", Pattern: "uname"},
	{Action: Allow, Tool: "bash", Pattern: "date"},
	{Action: Allow, Tool: "bash", Pattern: "tree"},
	// 搜索
	{Action: Allow, Tool: "bash", Pattern: "grep"},
	{Action: Allow, Tool: "bash", Pattern: "rg"},
	// git 只读操作
	{Action: Allow, Tool: "bash", Pattern: "git status"},
	{Action: Allow, Tool: "bash", Pattern: "git log"},
	{Action: Allow, Tool: "bash", Pattern: "git diff"},
	{Action: Allow, Tool: "bash", Pattern: "git show"},
	{Action: Allow, Tool: "bash", Pattern: "git branch"},
	{Action: Allow, Tool: "bash", Pattern: "git stash list"},
	// Go 工具只读
	{Action: Allow, Tool: "bash", Pattern: "go version"},
	{Action: Allow, Tool: "bash", Pattern: "go fmt"},
	{Action: Allow, Tool: "bash", Pattern: "go vet"},
	{Action: Allow, Tool: "bash", Pattern: "go doc"},
	// Docker 只读
	{Action: Allow, Tool: "bash", Pattern: "docker ps"},
	{Action: Allow, Tool: "bash", Pattern: "docker images"},
	{Action: Allow, Tool: "bash", Pattern: "docker logs"},
	{Action: Allow, Tool: "bash", Pattern: "docker inspect"},
	// 包管理器只读
	{Action: Allow, Tool: "bash", Pattern: "npm list"},
	{Action: Allow, Tool: "bash", Pattern: "npm outdated"},
	{Action: Allow, Tool: "bash", Pattern: "npm view"},
	{Action: Allow, Tool: "bash", Pattern: "pip list"},
	{Action: Allow, Tool: "bash", Pattern: "pip show"},
	{Action: Allow, Tool: "bash", Pattern: "pip freeze"},
}

// dangerCommands 可能修改系统/文件状态的命令，需用户确认
var dangerCommands = []Rule{
	// 文件操作
	{Action: Ask, Tool: "bash", Pattern: "rm"},
	{Action: Ask, Tool: "bash", Pattern: "mv"},
	{Action: Ask, Tool: "bash", Pattern: "cp"},
	{Action: Ask, Tool: "bash", Pattern: "dd"},
	{Action: Ask, Tool: "bash", Pattern: "mkdir"},
	{Action: Ask, Tool: "bash", Pattern: "touch"},
	{Action: Ask, Tool: "bash", Pattern: "ln"},
	{Action: Ask, Tool: "bash", Pattern: "chmod"},
	{Action: Ask, Tool: "bash", Pattern: "chown"},
	{Action: Ask, Tool: "bash", Pattern: "chattr"},
	// 网络操作
	{Action: Ask, Tool: "bash", Pattern: "curl"},
	{Action: Ask, Tool: "bash", Pattern: "wget"},
	{Action: Ask, Tool: "bash", Pattern: "nc"},
	{Action: Ask, Tool: "bash", Pattern: "netcat"},
	{Action: Ask, Tool: "bash", Pattern: "ssh"},
	{Action: Ask, Tool: "bash", Pattern: "scp"},
	{Action: Ask, Tool: "bash", Pattern: "rsync"},
	{Action: Ask, Tool: "bash", Pattern: "ftp"},
	// git 写操作
	{Action: Ask, Tool: "bash", Pattern: "git add"},
	{Action: Ask, Tool: "bash", Pattern: "git commit"},
	{Action: Ask, Tool: "bash", Pattern: "git push"},
	{Action: Ask, Tool: "bash", Pattern: "git reset"},
	{Action: Ask, Tool: "bash", Pattern: "git clean"},
	{Action: Ask, Tool: "bash", Pattern: "git rebase"},
	{Action: Ask, Tool: "bash", Pattern: "git merge"},
	// 包管理
	{Action: Ask, Tool: "bash", Pattern: "apt"},
	{Action: Ask, Tool: "bash", Pattern: "yum"},
	{Action: Ask, Tool: "bash", Pattern: "brew"},
	{Action: Ask, Tool: "bash", Pattern: "pip install"},
	{Action: Ask, Tool: "bash", Pattern: "npm install"},
	// 进程/系统
	{Action: Ask, Tool: "bash", Pattern: "kill"},
	{Action: Ask, Tool: "bash", Pattern: "pkill"},
	{Action: Ask, Tool: "bash", Pattern: "killall"},
	{Action: Ask, Tool: "bash", Pattern: "systemctl"},
	{Action: Ask, Tool: "bash", Pattern: "service"},
	{Action: Ask, Tool: "bash", Pattern: "shutdown"},
	{Action: Ask, Tool: "bash", Pattern: "reboot"},
	{Action: Ask, Tool: "bash", Pattern: "mount"},
	{Action: Ask, Tool: "bash", Pattern: "umount"},
}

// interpreterCommands 脚本解释器 — 无法静态分析脚本内容，需用户确认
var interpreterCommands = []Rule{
	{Action: Ask, Tool: "bash", Pattern: "python"},
	{Action: Ask, Tool: "bash", Pattern: "python3"},
	{Action: Ask, Tool: "bash", Pattern: "node"},
	{Action: Ask, Tool: "bash", Pattern: "nodejs"},
	{Action: Ask, Tool: "bash", Pattern: "go run"},
	{Action: Ask, Tool: "bash", Pattern: "bash"},
	{Action: Ask, Tool: "bash", Pattern: "sh"},
	{Action: Ask, Tool: "bash", Pattern: "zsh"},
	{Action: Ask, Tool: "bash", Pattern: "eval"},
	{Action: Ask, Tool: "bash", Pattern: "source"},
	{Action: Ask, Tool: "bash", Pattern: "ruby"},
	{Action: Ask, Tool: "bash", Pattern: "perl"},
	{Action: Ask, Tool: "bash", Pattern: "deno run"},
	{Action: Ask, Tool: "bash", Pattern: "bun run"},
}

// sensitiveFilePatterns 敏感文件匹配模式，供 read 和 grep 工具共享。
// 新增敏感文件类型只需在此列表追加一项，避免 read 和 grep 两边不同步。
var sensitiveFilePatterns = []string{
	".env*", "*.pem", "*.key", "*credentials*", "*secret*",
	"*token*", "*password*", "id_rsa*", "*.pfx", "*.p12", "*.jks",
}

// sensitiveFiles 敏感文件读取/搜索规则 — 由 sensitiveFilePatterns 按工具名生成
var sensitiveFiles []Rule

func init() {
	for _, tool := range []string{"read", "grep"} {
		for _, p := range sensitiveFilePatterns {
			sensitiveFiles = append(sensitiveFiles, Rule{Action: Ask, Tool: tool, Pattern: p})
		}
	}
}
