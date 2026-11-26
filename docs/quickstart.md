---
title: 快速开始
---

# 快速开始

安装 Docker 和 Docker Compose v2，从 [GitHub 仓库](https://github.com/shyun-cc/half-km) 获取源码，再进入源码根目录执行以下命令。

```sh
docker compose up --build -d
curl -fsS --retry 5 --retry-connrefused http://127.0.0.1:8080/health
curl -fsS http://127.0.0.1:8080/ready
```

端口有自定义配置时，将命令中的 `8080` 换成配置值。启动失败时，在源码根目录运行 `docker compose ps -a` 和 `docker compose logs` 查看服务状态与日志。

浏览器打开 `http://127.0.0.1:8080/`。首次启用业务时，先按[部署指南](deployment.md#启用业务角色)生成运营令牌并重启服务。用运营令牌创建社区、商户和团长后，可以发放商户、居民、团长令牌，按[操作流程](workflows.md)试用。

完成试用后，运行 `docker compose down` 停止服务并保留数据卷。参阅[部署指南](deployment.md)了解端口和数据卷设置。
