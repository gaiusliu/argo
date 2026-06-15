package deck

import (
	"strconv"
)

func postProcess(result ToolResult) ToolResult {
	outputLen := len(result.Output)

	if outputLen <= MaxOutputChars+TruncationMinSavings {
		return result
	}

	head := result.Output[:20000]
	tail := result.Output[outputLen-30000:]
	omitted := outputLen - MaxOutputChars

	marker := "\n\n[... 输出被截断，省略 " +
		strconv.Itoa(omitted) +
		" 字符，原始长度 " +
		strconv.Itoa(outputLen) +
		" 字符 ...]\n\n"

	result.Output = head + marker + tail
	return result
}
