package brig

// loadStaticRules 合并全部静态规则组，返回独立副本供 Engine 加载。
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
	{Action: Deny, Source: Static, Tool: "bash", Pattern: "rm -rf /"},
	{Action: Deny, Source: Static, Tool: "bash", Pattern: "mkfs"},
	{Action: Deny, Source: Static, Tool: "bash", Pattern: "dd if="},
	{Action: Deny, Source: Static, Tool: "bash", Pattern: ":(){ :|:& };:"},

	// SSRF 防护 — 禁止访问内网地址
	{Action: Deny, Source: Static, Tool: "web_fetch", Pattern: "127.0.0.1"},
	{Action: Deny, Source: Static, Tool: "web_fetch", Pattern: "localhost"},
	{Action: Deny, Source: Static, Tool: "web_fetch", Pattern: "0.0.0.0"},

	// 系统关键文件 — 拒绝写入
	{Action: Deny, Source: Static, Tool: "write", Pattern: "/etc/shadow"},
	{Action: Deny, Source: Static, Tool: "edit", Pattern: "/etc/shadow"},
	{Action: Deny, Source: Static, Tool: "write", Pattern: "C:\\Windows\\System32"},
	{Action: Deny, Source: Static, Tool: "edit", Pattern: "C:\\Windows\\System32"},
}

// safeCommands 纯读取/查询类 bash 命令，直接 Allow
var safeCommands = []Rule{
	// 文件查看
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "ls"},
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "cat"},
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "head"},
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "tail"},
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "find"},
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "wc"},
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "du"},
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "df"},
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "stat"},
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "file"},
	// 系统信息
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "pwd"},
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "echo"},
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "printf"},
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "sort"},
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "uniq"},
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "cut"},
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "tr"},
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "awk"},
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "which"},
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "whereis"},
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "type"},
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "whoami"},
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "hostname"},
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "uname"},
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "date"},
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "tree"},
	// 搜索
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "grep"},
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "rg"},
	// git 只读操作
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "git status"},
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "git log"},
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "git diff"},
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "git show"},
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "git branch"},
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "git stash list"},
	// Go 工具只读
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "go version"},
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "go fmt"},
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "go vet"},
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "go doc"},
	// Docker 只读
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "docker ps"},
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "docker images"},
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "docker logs"},
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "docker inspect"},
	// 包管理器只读
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "npm list"},
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "npm outdated"},
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "npm view"},
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "pip list"},
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "pip show"},
	{Action: Allow, Source: Static, Tool: "bash", Pattern: "pip freeze"},
}

// dangerCommands 可能修改系统/文件状态的命令，需用户确认
var dangerCommands = []Rule{
	// 文件操作
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "rm"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "mv"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "cp"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "dd"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "mkdir"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "touch"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "ln"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "chmod"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "chown"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "chattr"},
	// 网络操作
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "curl"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "wget"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "nc"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "netcat"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "ssh"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "scp"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "rsync"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "ftp"},
	// git 写操作
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "git add"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "git commit"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "git push"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "git reset"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "git clean"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "git rebase"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "git merge"},
	// 包管理
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "apt"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "yum"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "brew"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "pip install"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "npm install"},
	// 进程/系统
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "kill"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "pkill"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "killall"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "systemctl"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "service"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "shutdown"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "reboot"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "mount"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "umount"},
}

// interpreterCommands 脚本解释器 — 无法静态分析脚本内容，需用户确认
var interpreterCommands = []Rule{
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "python"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "python3"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "node"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "nodejs"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "go run"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "bash"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "sh"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "zsh"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "eval"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "source"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "ruby"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "perl"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "deno run"},
	{Action: Ask, Source: Static, Tool: "bash", Pattern: "bun run"},
}

// sensitiveFiles 敏感文件路径模式 — read/grep 命中时需用户确认
var sensitiveFiles = []Rule{
	{Action: Ask, Source: Static, Tool: "read", Pattern: ".env*"},
	{Action: Ask, Source: Static, Tool: "read", Pattern: "*.pem"},
	{Action: Ask, Source: Static, Tool: "read", Pattern: "*.key"},
	{Action: Ask, Source: Static, Tool: "read", Pattern: "*credentials*"},
	{Action: Ask, Source: Static, Tool: "read", Pattern: "*secret*"},
	{Action: Ask, Source: Static, Tool: "read", Pattern: "*token*"},
	{Action: Ask, Source: Static, Tool: "read", Pattern: "*password*"},
	{Action: Ask, Source: Static, Tool: "read", Pattern: "id_rsa*"},
	{Action: Ask, Source: Static, Tool: "read", Pattern: "*.pfx"},
	{Action: Ask, Source: Static, Tool: "read", Pattern: "*.p12"},
	{Action: Ask, Source: Static, Tool: "read", Pattern: "*.jks"},
	{Action: Ask, Source: Static, Tool: "grep", Pattern: ".env*"},
	{Action: Ask, Source: Static, Tool: "grep", Pattern: "*.pem"},
	{Action: Ask, Source: Static, Tool: "grep", Pattern: "*.key"},
	{Action: Ask, Source: Static, Tool: "grep", Pattern: "*credentials*"},
	{Action: Ask, Source: Static, Tool: "grep", Pattern: "*secret*"},
	{Action: Ask, Source: Static, Tool: "grep", Pattern: "*token*"},
	{Action: Ask, Source: Static, Tool: "grep", Pattern: "*password*"},
	{Action: Ask, Source: Static, Tool: "grep", Pattern: "id_rsa*"},
	{Action: Ask, Source: Static, Tool: "grep", Pattern: "*.pfx"},
	{Action: Ask, Source: Static, Tool: "grep", Pattern: "*.p12"},
	{Action: Ask, Source: Static, Tool: "grep", Pattern: "*.jks"},
}
