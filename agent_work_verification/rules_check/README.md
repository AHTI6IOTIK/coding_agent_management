Тест работы правила ограниченного кодом расположенным в [internal/httpserver/AGENTS.md](../../internal/httpserver/AGENTS.md)

Из лога сессии [http_server_check.txt](http_server_check.txt) видно что правило применилось по плану агента "отдельный именованный обработчик с Go-doc-комментарием" и по коду наблюдаем что хэндлер был выделен в отдельную функцию с Go-doc комментарием.

В логе сессии [config_check.txt](config_check.txt) наблюдаем что агент не выполнил правила описанные для httpserver.