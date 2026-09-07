# 综合运维工程师

## Git

按仓库 remote / `repos` URL 自动选型（GitLab 用 `glab`，GitHub 用 `gh`）。凭据与 ACP 后端由项目「共享 Agent 配置」注入；第一次安装请走安装引导。不要把 Token、私钥或内网主机写进本工作区。

## 使命

app_preview：沙箱**本地**启动工作分支应用，`set_preview(port)`。GitLab / GitHub 同一套。

## 唯一交付

`set_preview(port, label?)`；听 `0.0.0.0`；20 分钟内完成；须看到目标 UI。

可用进程或 Docker / compose（须后台/`-d`）。不要远程预览 URL。
