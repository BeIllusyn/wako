# wako

[English](README.md) | [简体中文](README_ZH.md)

一个记住如何启动你的服务的小型 CLI。

只需注册一次服务——命令和运行目录——之后就能在任何地方用一条命令启动、停止、重启和接管它。

```console
$ wako add web pnpm dev
added service "web": pnpm dev (in /Users/you/code/web)

$ wako start web
wako: "web" running as "web": pnpm dev (in /Users/you/code/web)
...
```

## 特性

- **一条命令启动任何东西** —— wako 记住命令和工作目录，你不再需要到处 `cd`。
- **前台或后台** —— 保持接看输出，或者分离后让服务继续运行。
- **随时接管与分离** —— `wako resume` 重新接管正在运行的服务；`Ctrl+C` 只分离、不停止服务。
- **带名字的运行实例** —— 同一个服务可以运行多次，每个实例都会得到一个唯一的名字（`web`、`web-1`、`web-2`……），也可以用 `-n` 自己指定。
- **没有守护进程** —— 每个运行实例由它自己的一个微型 supervisor 进程负责。
- **跨平台** —— 支持 macOS、Linux 和 Windows。

## 安装

需要 [Go](https://go.dev) 1.26 或更高版本。

```sh
git clone https://github.com/liuenzuo666/wako.git
cd wako
go install .
```

确保 `$(go env GOPATH)/bin` 在你的 `PATH` 中。或者，直接把二进制构建到任意位置：

```sh
go build -o wako .
```

## 用法

### 注册服务

在服务所在的目录中运行 `wako add`：

```sh
cd ~/code/web
wako add web pnpm dev
```

名字后面的所有内容都是命令，所以不需要加引号：

```sh
wako add api go run ./cmd/server --port 8080
```

用 `-r` 替换已有服务，用 `wako remove <name>` 删除服务。

### 启动服务

```sh
wako start web        # 前台：输出直接流到你的终端
wako start -d web     # 后台：wako 退出后继续运行
```

接看时按 `Ctrl+C` 会停止服务。已分离的运行实例会继续运行，用下面的命令重新接看：

```sh
wako resume web
```

在 resume 时按 `Ctrl+C` 只会分离，不会停止服务。

### 多次运行同一个服务

每个运行实例都会得到一个唯一的名字：服务名，或者在被占用时使用 `<服务名>-1`、`<服务名>-2`……也可以用 `-n` 自己指定：

```sh
wako start -d web          # 运行实例 "web"
wako start -d web          # 运行实例 "web-1"
wako start -d -n web2 web  # 运行实例 "web2"
```

### 管理运行实例

```sh
wako ps              # 列出正在运行的服务及其运行实例名
wako stop web-1      # 停止某个运行实例
wako stop --all      # 停止全部
wako restart web     # 停止 "web" 并重新启动
wako restart -d web  # 在后台重启
```

`restart` 使用服务当前的命令和目录，所以通过 `wako add -r` 做的修改会在下次重启时生效。

### 管理服务

```sh
wako list      # 列出已注册的服务名
wako list -a   # 同时显示命令和目录
```

## 命令

| 命令 | 说明 |
| --- | --- |
| `wako add [-r] <name> <command>` | 注册服务（记住当前目录） |
| `wako remove <name>` | 删除服务（别名 `rm`） |
| `wako start [-d] [-n <run name>] <name>` | 启动服务 |
| `wako stop [--all] [run name]` | 停止某个运行实例，或停止全部 |
| `wako restart [-d] [-n <run name>] <name>` | 用当前命令重启服务 |
| `wako resume <run name>` | 接管正在运行的服务（`Ctrl+C` 分离） |
| `wako ps` | 列出正在运行的服务（别名 `runs`） |
| `wako list [-a]` | 列出服务；`-a` 同时显示命令和目录（别名 `ls`） |
| `wako help` | 显示帮助 |

## 工作原理

`wako start` 会先独占一个运行实例名，然后启动一个隐藏的 supervisor 进程（`wako __supervise -n <run name>`）。这个进程把命令放在它自己的进程组里运行，并在整个生命周期内负责该实例。supervisor 会在内存中保留最近 256 KB 的输出，所以即使稍晚一点接管，也能看到启动时的输出。

客户端通过控制套接字与 supervisor 通信——macOS 和 Linux 上是 Unix socket，Windows 上是回环 TCP 端口。第一帧必须包含运行实例状态文件中的随机 token，因此只有能读取该文件（权限 `0600`）的进程才能接管或停止实例。

服务退出后，supervisor 会短暂停留，把退出码交给即将接管的客户端，然后清理实例的状态文件和套接字并退出。这里没有守护进程：停止所有实例后不会留下任何东西。

## 文件

| 路径 | 用途 |
| --- | --- |
| `~/.wako/config.json` | 已注册的服务 |
| `~/.wako/runs/*.json` | 存活实例的状态（结束时删除） |
| `~/.wako/runs/*.sock` | 控制套接字（Unix） |

设置 `WAKO_CONFIG` 可以覆盖配置文件位置，`runs` 目录也会随之移动（方便测试）。

## 开发

```sh
gofmt -l .
go vet ./...
go build ./...
```

## 许可证

[MIT](LICENSE)
