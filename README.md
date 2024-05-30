# Ollama

## Linux LoongArch Version

目前处于测试阶段，目的是将来可用后，完整合并到 ollama 主线，目前 `go build .` 阶段无法通过，有兴趣的朋友可以加入一起玩。提供您的想法。

1. 首先克隆本项目到本地：
```
git clone https://github.com/HougeLangley/ollama-loongarch64
```
2. 进入该项目并初始化 llama.cpp 模块：
```
cd ollama-loongarch64; git submodule init; git submodule update
```
3. 开始构建：
```
go generate ./...
```
```
go build .
```
4. 目前 `go build .` 会因为失败退出。暂时没有找到原因，也请大家多多帮忙测试和提供信息。

目前提示的错误信息是：

```
# github.com/ollama/ollama
/usr/lib/go/pkg/tool/linux_loong64/link: running loongarch64-unknown-linux-gnu-gcc failed: exit status 1
/usr/lib/gcc/loongarch64-unknown-linux-gnu/14/../../../../loongarch64-unknown-linux-gnu/bin/ld: /tmp/go-link-2425035515/000020.o: in function `_cgo_bc67fad7ee33_Cfunc_llama_model_quantize':
/tmp/go-build/llm.cgo2.c:66:(.text+0x34): undefined reference to `llama_model_quantize'
/usr/lib/gcc/loongarch64-unknown-linux-gnu/14/../../../../loongarch64-unknown-linux-gnu/bin/ld: /tmp/go-link-2425035515/000020.o: in function `_cgo_bc67fad7ee33_Cfunc_llama_model_quantize_default_params':
/tmp/go-build/llm.cgo2.c:83:(.text+0x9c): undefined reference to `llama_model_quantize_default_params'
/usr/lib/gcc/loongarch64-unknown-linux-gnu/14/../../../../loongarch64-unknown-linux-gnu/bin/ld: /tmp/go-link-2425035515/000020.o: in function `_cgo_bc67fad7ee33_Cfunc_llama_print_system_info':
/tmp/go-build/llm.cgo2.c:100:(.text+0x110): undefined reference to `llama_print_system_info'
collect2: 错误：ld 返回 1
```
