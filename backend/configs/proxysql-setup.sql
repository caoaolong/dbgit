-- dbgit 本地 ProxySQL 路由配置（最终版：按前端账号分流）
-- 后端：dbgit-mysql57:3306 (5.7, hostgroup 10) / dbgit-mysql80:3306 (8.0, hostgroup 20)，同属 dbgit-net 网络
-- 注意：ProxySQL 3.0.11 中 schemaname 规则不生效，故采用“一人一路由”方案；
-- 后端 MySQL 上需存在同名账号（已创建 app57/app80，密码 dbgit-app-123）

-- 1. 后端节点
DELETE FROM mysql_servers WHERE hostgroup_id IN (10, 20);
INSERT INTO mysql_servers (hostgroup_id, hostname, port, weight) VALUES
  (10, 'dbgit-mysql57', 3306, 100),
  (20, 'dbgit-mysql80', 3306, 100);

-- 2. 前端账号（default_hostgroup 决定路由）
DELETE FROM mysql_users WHERE username IN ('root', 'app57', 'app80');
INSERT INTO mysql_users (username, password, default_hostgroup, transaction_persistent) VALUES
  ('root',  'dbgit-root-123', 10, 1),
  ('app57', 'dbgit-app-123',  10, 1),
  ('app80', 'dbgit-app-123',  20, 1);

-- 3. 清理实验性 query rules（本版本 schemaname 匹配无效，不用它）
DELETE FROM mysql_query_rules;

-- 4. 生效并持久化
LOAD MYSQL SERVERS TO RUNTIME;
LOAD MYSQL USERS TO RUNTIME;
LOAD MYSQL QUERY RULES TO RUNTIME;
SAVE MYSQL SERVERS TO DISK;
SAVE MYSQL USERS TO DISK;
SAVE MYSQL QUERY RULES TO DISK;
