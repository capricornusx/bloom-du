[![Lint](https://github.com/capricornusx/bloom-du/actions/workflows/lint.yml/badge.svg)](https://github.com/capricornusx/bloom-du/actions/workflows/lint.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/capricornusx/bloom-du)](https://goreportcard.com/report/github.com/capricornusx/bloom-du)
[![codecov](https://codecov.io/gh/capricornusx/bloom-du/graph/badge.svg?token=IO7ABLPOFP)](https://codecov.io/gh/capricornusx/bloom-du)

### Bloom-du (HTTP API для фильтра Блума)

#### 0. Установка

 - Бинарный файл для вашей платформы (.deb, .rpm, .gz) 
 - Docker image `ghcr.io/capricornusx/bloom-du`

```sh
docker run --rm -v tmpsf:/var/lib/bloom-du/ \
-p 8515:8515 -it \
ghcr.io/capricornusx/bloom-du --log_level=info
```

[docker-compose.yml](docs/docker-compose.yml)

Добавить данные в фильтр можно несколькими способами:
#### 1. Загрузка из файла (импорт)
В папке с бинарным файлом разместить текстовый файл (например, `values.txt`), с одним значением на строку. 
После запустить: 
 
```sh
bloom-du --log_level=debug --source=values.txt -o sbfData.bloom
```

Через некоторое время все строки будут загружены в фильтр. Далее исходный файл уже не нужен. 
Вся структура фильтра будет сохранена в файл `sbfData.bloom`. Последующие запуски весь фильтр 
будет загружен из этого файла. Если его удалить, придётся снова его наполнять нужными вам данными.

Поддерживается загрузка значений сразу из .gz файла (без его распаковки):

```sh
bloom-du --source=values.txt.gz
```

#### 2. Загрузка через API
Загрузить каждое значение поштучно через API (пока нет bulk загрузки, через API):

```sh
curl -X POST --location "http://localhost:8515/api/add?value=blablabla" \
-H "Accept: application/json" 
```


Проверить наличие элемента в фильтре:
```sh
curl -X GET --location "http://localhost:8515/api/check?value=12344" \
    -H "Accept: application/json"
```


Метрики, которые можно собирать через Prometheus имеют префикс `bloom_du_*`, например:


```sh
curl -X GET --location "http://localhost:8515/metrics"
```

 - `bloom_du_config_info`
 - `bloom_du_elements_total`
 - `bloom_du_api_http_request_duration_seconds`

Кроме этого, есть стандартные метрики, которые отдаёт Go.

Дашборд для Grafana - [grafana-bloom-du.json](internal%2Futils%2Fgrafana-bloom-du.json)


### TODO
- [ ] Возможность создавать разные фильтры (название, настройки размера, fpRate ...)
- [ ] Graceful upgrade - обновление самого бинарника и корректная обработка клиентов (старых и новых)
- [ ] config.yml для удобного старта сервиса с разными фильтрами
- [ ] Валидация параметров для cli и api
- [ ] code coverage


### Тесты (redis_version:7.2.3)

```redis
BF.RESERVE, "key", 0.1, 200000000
```

Скорость работы в однопоточном режиме:

| Case                               | RPS     |
|------------------------------------|---------|
| Нативно в Redis  [1](#native-case) | ~12 190 |
| Реализация на Go [2](#golang-case) | ~15 705 |

```math
(15705 / 12190) * 100 = 128,8%
```

### native-case
Клиентом на Go (github.com/redis/go-redis/v9), проверяем наличие ключа.
Отправляем 1кк запросов в один поток. Redis работает в docker, на том же ПК.

```redis
BF.EXISTS, "key", "some_value"
``` 

### golang-case
С помощью Apache benchmark запрос по http:

```sh
ab -k -i -n 1000000 -c 1 "http://localhost:8515/api/fcheck?value=some_value"
```

### Пример

На своём проекте, есть проверка входящих данных на дубликаты. Причём мы всё равно должны записать эти данные в базу, 
поэтому нельзя создать уникальный индекс, переложив процесс валидации на БД. Хотя всё равно необходимо наличие индекса 
по искомому полю, чтобы в случае НЕ отрицательного ответа из фильтра, уже точно идти в БД. Нам не важен `fpRate`, 
т.к. достаточно 100% отсутствия элемента. 
Эффективность очень сильно зависит от соотношения оригиналов/дубликатов. В этом случае мы экономим обращениях в БД.
Для наших задач подошло хорошо.

```php
public function isDouble(string $element): bool
{
   $isExist = findElementInFilter($element);

   if ($isExist === true) {
      // Элемент возможно есть фильтре, идём в БД чтобы проверить точно
      $isExist = findElementInDatabase($element);
   }

   return $isExist;
}

$isDouble = isDouble($element);

if ($isDouble === false) {
   addElementToFilter($element);
   addElementToDatabase($element);
}
```

### Ссылки, источники:

1. Нечто похожее есть в Postgresql - но реализация кажется отличается. Требует исследования.
   - [BRIN индекс](https://postgrespro.ru/docs/postgresql/16/brin-builtin-opclasses)
   - [bloom](https://postgrespro.ru/docs/postgresql/15/bloom)
   - [Bloom Indexes in PostgreSQL](https://www.percona.com/blog/bloom-indexes-in-postgresql/)
   - [Habr - Индексы в PostgreSQL 10](https://habr.com/ru/companies/postgrespro/articles/349224)
   - [Roaring Bitmaps and pgfaceting: Fast counting across large datasets in Postgres](https://pganalyze.com/blog/5mins-postgres-roaring-bitmaps-pgfaceting-query-performance)
   - [pgfaceting](https://github.com/cybertec-postgresql/pgfaceting)
   - [pg_roaringbitmap](https://github.com/ChenHuajun/pg_roaringbitmap)

2. Redis. Поддерживается только классическая реализация фильтра
   - [Redis data-types](https://redis.io/docs/data-types/probabilistic/bloom-filter/)
   - [RedisBloom](https://github.com/RedisBloom/RedisBloom) модуль, входящий в состав Redis Stack
   - [Redis bitmaps – Fast, easy, realtime metrics](https://spoolblog.wordpress.com/2011/11/29/fast-easy-realtime-metrics-using-redis-bitmaps/)
   - [Using Probabilistic Data Structures in Redis](https://semaphoreci.com/blog/probabilistic-data-structures-redis)
   - [Understanding Probabilistic Data Structures](https://github.com/guyroyse/understanding-probabilistic-data-structures)
   - [Bloom Filter Calculator](https://hur.st/bloomfilter) Калькулятор для классической реализации

3. [Библиотека](https://github.com/tylertreat/BoomFilters) на Go, которая использована в этом проект как основная.
Содержит в себе реализации стабильного фильтра Блума и других Probabilistic (вероятностных) структур.
4. [Bitmap-индексы в Go: поиск на дикой скорости](https://habr.com/ru/companies/badoo/articles/451938/) Крутая статья и комменты о схожей теме.
5. [Probabilistic Data Structures for Web Analytics and Data Mining](https://highlyscalable.wordpress.com/2012/05/01/probabilistic-structures-web-analytics-data-mining/)
6. [Roaring bitmaps - A better compressed bitset](https://roaringbitmap.org/about/)
7. [Daniel Lemire](https://github.com/lemire) is a computer science professor. Roaring bitmaps contributor
8. [Probabilistic Data Structures and Algorithms](https://github.com/gakhov)


