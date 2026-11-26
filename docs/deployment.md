---
title: 部署指南
---

# 部署指南

使用仓库自带的 Compose 配置在本机部署。首次启动步骤见[快速开始](quickstart.md)。

- PostgreSQL 18 在首次创建数据卷时初始化数据库结构。
- API 绑定到本机回环地址，使用 `http://127.0.0.1:8080` 访问。
- `/health` 检查 API 进程，`/ready` 检查数据库连接。

维护已有实例时，先备份 PostgreSQL，再按目标版本的升级说明操作。停止实例时使用 `docker compose down`，这样可以保留数据卷。
