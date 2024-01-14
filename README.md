[![Go](https://github.com/capricornusx/bloom-du/actions/workflows/go.yml/badge.svg)](https://github.com/capricornusx/bloom-du/actions/workflows/go.yml)
[![Lint](https://github.com/capricornusx/bloom-du/actions/workflows/lint.yml/badge.svg)](https://github.com/capricornusx/bloom-du/actions/workflows/lint.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/capricornusx/bloom-du)](https://goreportcard.com/report/github.com/capricornusx/bloom-du)

### Bloom-du (HTTP API для фильтра Блума)

Добавить данные в фильтр можно несколькими способами:
#### 1. Загрузка из файла (импорт)
В папке с бинарным файлом разместить текстовый файл (например, values.txt), с одним значением на строку. 
После запустить 
 
`bloom-du --source values.txt` 

Через некоторое время все строки будут загружены в фильтр. Далее исходный файл уже не нужен. 
Вся структура фильтра будет сохранена в файл `sbfData.bloom`. Последующие запуски весь фильтр 
будет загружен из этого файла. Если его удалить, придётся снова его наполнять нужными вам данными.


#### 2. Загрузка через API
Загрузить каждое значение поштучно через API (пока нет bulk загрузки, через API):

```sh
curl -X POST --location "http://localhost:8515/api/add?value=blablabla" \
-H "Accept: application/json" 
```
Ещё примеры использования API смотрите в файле [api.http](internal%2Fapi%2Fapi.http)



### Ссылки, источники:

1. Похожий индекс есть в Postgresql - но реализация кажется отличается.
   - [BRIN индекс](https://postgrespro.ru/docs/postgresql/16/brin-builtin-opclasses)
   - [bloom](https://postgrespro.ru/docs/postgresql/15/bloom)
   - [Habr - Индексы в PostgreSQL 10](https://habr.com/ru/companies/postgrespro/articles/349224/)
2. Redis с поддержкой это структуры. Но Redis показывает существенно больший расход
   памяти при равных стартовых условиях. Возможно потому что используется простая реализация фильтра, а здесь Стабильная.
   В общем, требует исследования. Может я редис не правильно использовал.
   - [Redis Stack](https://redis.io/docs/data-types/probabilistic/bloom-filter/)
      Вариант редиса, который имеет из коробки, в том числе фильтр Блума.
   - [RedisBloom](https://github.com/RedisBloom/RedisBloom) Github модуля, входящего в состав Redis Stack
3. [Библиотека](github.com/tylertreat/BoomFilters) на Go, которая использована в этом проект как основная.
Содержит в себе реализации стабильного фильтра Блума и других Probabilistic (вероятностных) структур.






#### TODO:
- [ ] Для продакшена нужна миграция данных из работающей БД в Filter. Наверное лучше всего подойдёт курсор
  и выборка данных по дате создания сущности. 
- [x] Попробовать использовать контексты (для отмены по сигналам и вообще)
- [ ] *Возможно - валидация номера в query параметре
- [ ] *Возможно - авторизация, хотя бы по токену

