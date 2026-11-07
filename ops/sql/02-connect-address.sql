-- 客户端 SDK 通过 Navigator 服务拿到的长连接地址。
-- 首次启动前把 im.example.com 改成真实域名；之后修改只需重新执行本语句并重启引擎。
--
-- 值格式为 host:port，不带协议前缀。走 Nginx 443 时端口写 443。

SET @conf_value = '{"default":["im.example.com:443"]}';

INSERT IGNORE INTO `globalconfs` (`conf_key`, `conf_value`)
VALUES ('connect_address', '');

UPDATE `globalconfs` SET `conf_value` = @conf_value
WHERE `conf_key` = 'connect_address';

SELECT `conf_key`, `conf_value` FROM `globalconfs` WHERE `conf_key` = 'connect_address';
