---
title: 部署指南
---

# 部署指南

使用仓库自带的 Compose 配置在本机部署。首次启动步骤见[快速开始](quickstart.md)。

- PostgreSQL 18 在首次创建数据卷时按文件名顺序执行迁移。
- API 绑定到本机回环地址，使用 `http://127.0.0.1:8080` 访问。
- `/health` 检查 API 进程，`/ready` 检查数据库与业务结构是否就绪。
- `docker compose down` 停止实例并保留数据卷。

## 升级里程碑 1 的实例

先备份 PostgreSQL。在运行中的 Compose 数据库上应用第二个迁移，然后重新启动 API。

```sh
docker compose stop api
docker compose exec -T db psql -U halfkm -d halfkm -v ON_ERROR_STOP=1 < migrations/0002_core_workflows.sql
docker compose up --build -d
curl -fsS http://127.0.0.1:8080/ready
```

使用独立的新数据库时，从仓库根目录通过已配置的 PostgreSQL 连接运行以下命令。既有实例只应用尚未执行的文件。

```sh
for migration in migrations/*.sql; do
  psql -v ON_ERROR_STOP=1 -f "$migration" || break
done
```

## 启用业务角色

在仓库之外保存权限配置文件，通过 `HALFKM_AUTH_FILE` 指定其绝对路径。每个角色使用独立随机令牌。使用 `openssl rand -hex 32` 生成令牌，再对原始令牌做 SHA-256 摘要。只把摘要写进配置，把原始令牌交给其持有人。另生成一份 32 字节随机密钥填写 `pickup_key_hex`，跨重启保留原值，使已签发提货码可继续使用。

```json
{
  "pickup_key_hex": "<32 字节密钥的十六进制字符串>",
  "tokens": [
    { "role": "operator", "token_sha256": "<运营令牌摘要>" },
    { "role": "merchant", "id": 1, "token_sha256": "<商户令牌摘要>" },
    { "role": "payments", "id": 1, "token_sha256": "<收款与退款登记令牌摘要>" },
    { "role": "leader", "id": 1, "token_sha256": "<团长令牌摘要>" },
    { "role": "buyer", "buyer_id": "<买家 UUID>", "token_sha256": "<买家令牌摘要>" }
  ]
}
```

可先只配置运营角色。创建商户和团长后，用返回的实际编号配置相应角色。为每位买家分配不同的 UUID。修改文件后重启 API。客户端使用原始令牌登录业务接口。

在 Compose 中启用业务权限时，在本地覆盖文件里把配置以只读方式挂载到 `api` 服务，设置 `HALFKM_AUTH_FILE` 指向容器内路径，并确保容器运行用户能读取该文件。共享部署应经 HTTPS 访问，数据库应使用实例自己的认证设置。
