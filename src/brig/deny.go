package brig

// BuiltinRuleLoader 加载内置静态规则。
type BuiltinRuleLoader struct{}

// Load 实现 RuleLoader 接口，按 Iron→Cord 顺序返回规则。
func (l BuiltinRuleLoader) Load() []Rule {
	var all []Rule

	// Iron：不可绕过的红线（原 denyRules）
	all = append(all, denyRules...)
	// Cord：可配置规则组
	all = append(all, safeCommands...)
	all = append(all, dangerCommands...)
	all = append(all, interpreterCommands...)
	all = append(all, sensitiveFiles...)

	return all
}

// denyRules 不可绕过的铁律规则（Iron）
var denyRules = []Rule{
	// 破坏性命令
	{Action: Deny, Tool: "bash", Pattern: "rm -rf /", Type: RuleIron},
	{Action: Deny, Tool: "bash", Pattern: "rm -rf /*", Type: RuleIron},
	{Action: Deny, Tool: "bash", Pattern: "mkfs", Type: RuleIron},
	{Action: Deny, Tool: "bash", Pattern: "dd if=", Type: RuleIron},
	{Action: Deny, Tool: "bash", Pattern: ":(){ :|:& };:", Type: RuleIron},

	// SSRF 防护 — 禁止访问内网地址
	{Action: Deny, Tool: "web_fetch", Pattern: "127.0.0.1", Type: RuleIron},
	{Action: Deny, Tool: "web_fetch", Pattern: "localhost", Type: RuleIron},
	{Action: Deny, Tool: "web_fetch", Pattern: "0.0.0.0", Type: RuleIron},

	// 系统关键文件 — 拒绝写入
	{Action: Deny, Tool: "write", Pattern: "/etc/shadow", Type: RuleIron},
	{Action: Deny, Tool: "edit", Pattern: "/etc/shadow", Type: RuleIron},
	{Action: Deny, Tool: "write", Pattern: "C:\\Windows\\System32", Type: RuleIron},
	{Action: Deny, Tool: "edit", Pattern: "C:\\Windows\\System32", Type: RuleIron},
}

// safeCommands 纯读取/查询类 bash 命令，直接 Allow
var safeCommands = []Rule{
	// 文件查看
	{Action: Approve, Tool: "bash", Pattern: "ls"},
	{Action: Approve, Tool: "bash", Pattern: "cat"},
	{Action: Approve, Tool: "bash", Pattern: "head"},
	{Action: Approve, Tool: "bash", Pattern: "tail"},
	{Action: Approve, Tool: "bash", Pattern: "find"},
	{Action: Approve, Tool: "bash", Pattern: "wc"},
	{Action: Approve, Tool: "bash", Pattern: "du"},
	{Action: Approve, Tool: "bash", Pattern: "df"},
	{Action: Approve, Tool: "bash", Pattern: "stat"},
	{Action: Approve, Tool: "bash", Pattern: "file"},
	// 系统信息
	{Action: Approve, Tool: "bash", Pattern: "pwd"},
	{Action: Approve, Tool: "bash", Pattern: "echo"},
	{Action: Approve, Tool: "bash", Pattern: "printf"},
	{Action: Approve, Tool: "bash", Pattern: "sort"},
	{Action: Approve, Tool: "bash", Pattern: "uniq"},
	{Action: Approve, Tool: "bash", Pattern: "cut"},
	{Action: Approve, Tool: "bash", Pattern: "tr"},
	{Action: Approve, Tool: "bash", Pattern: "awk"},
	{Action: Approve, Tool: "bash", Pattern: "which"},
	{Action: Approve, Tool: "bash", Pattern: "whereis"},
	{Action: Approve, Tool: "bash", Pattern: "type"},
	{Action: Approve, Tool: "bash", Pattern: "whoami"},
	{Action: Approve, Tool: "bash", Pattern: "hostname"},
	{Action: Approve, Tool: "bash", Pattern: "uname"},
	{Action: Approve, Tool: "bash", Pattern: "date"},
	{Action: Approve, Tool: "bash", Pattern: "tree"},
	// 搜索
	{Action: Approve, Tool: "bash", Pattern: "grep"},
	{Action: Approve, Tool: "bash", Pattern: "rg"},
	// git 只读操作
	{Action: Approve, Tool: "bash", Pattern: "git status"},
	{Action: Approve, Tool: "bash", Pattern: "git log"},
	{Action: Approve, Tool: "bash", Pattern: "git diff"},
	{Action: Approve, Tool: "bash", Pattern: "git show"},
	{Action: Approve, Tool: "bash", Pattern: "git branch"},
	{Action: Approve, Tool: "bash", Pattern: "git stash list"},
	// Go 工具只读
	{Action: Approve, Tool: "bash", Pattern: "go version"},
	{Action: Approve, Tool: "bash", Pattern: "go fmt"},
	{Action: Approve, Tool: "bash", Pattern: "go vet"},
	{Action: Approve, Tool: "bash", Pattern: "go doc"},
	// Docker 只读
	{Action: Approve, Tool: "bash", Pattern: "docker ps"},
	{Action: Approve, Tool: "bash", Pattern: "docker images"},
	{Action: Approve, Tool: "bash", Pattern: "docker logs"},
	{Action: Approve, Tool: "bash", Pattern: "docker inspect"},
	// 包管理器只读
	{Action: Approve, Tool: "bash", Pattern: "npm list"},
	{Action: Approve, Tool: "bash", Pattern: "npm outdated"},
	{Action: Approve, Tool: "bash", Pattern: "npm view"},
	{Action: Approve, Tool: "bash", Pattern: "pip list"},
	{Action: Approve, Tool: "bash", Pattern: "pip show"},
	{Action: Approve, Tool: "bash", Pattern: "pip freeze"},
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
