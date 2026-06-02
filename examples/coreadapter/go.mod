module github.com/costa92/llm-agent-memory-client/examples/coreadapter

go 1.26.0

require (
	github.com/costa92/llm-agent v0.7.0
	github.com/costa92/llm-agent-memory-client v0.0.0
)

require github.com/costa92/llm-agent-contract v0.0.0 // indirect

replace github.com/costa92/llm-agent-memory-client => ../..

replace github.com/costa92/llm-agent => ../../../llm-agent

replace github.com/costa92/llm-agent-contract => ../../../llm-agent-contract
