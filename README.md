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
4. 源码目录下会有 ollama ，即可运行测试。
