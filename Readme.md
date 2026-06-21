网页聊天系统

启动位置:cmd/server/main.go
使用mysql.sql中的语句创建表单
目前实现：
- 单对单通信，群组聊天室通信
- 注册与登录
- 修改/显示头像
- 使用MySQL实现消息记录功能
- 使用redis实现对在线状态的查询
- 通过cookie实现登陆状态的持久化

前端地址：https://github.com/REDDDG/testFront

# config.json
```json
{
"server": {
"port": ":9090"
},
"mysql": {
"user": "用户名",
"password": "密码",
"host": "localhost",
"port": 3306,
"database": "goland"
},
"redis": {
"addr": "localhost:6379",
"db": 0
},
"session": {
"secret": "密钥"
},
"cors": {
"origin": "http://localhost:8082"
}
}
```

