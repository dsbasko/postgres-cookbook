# Story uplift 2/5 — волна II: модули 04–06 (Акт II «Вместе»)

## Overview

Третий план семейства story-uplift (архив исходного плана —
`docs/plans/completed/20260703-story-uplift.md`). Раскатка сюжетного слоя на
модули 04 (кроме 04-01 — сделан в пилоте), 05 и 06 — 17 юнитов Акта II:
проблемы решаются вместе, читатель — полноправный участник.

**Предусловие запуска:** план `1-wave-i` завершён и принят владельцем.

Ключевые биты волны (детали — в записях карты сцен): развод камео Руслана и
staff-Анны (04-02; склейка §10.2 расторгнута — камео носит отдельное имя,
Анна остаётся только персонажем данных), «неделя инцидентов» конкурентности
(модуль 05), рифма постмортемов 02-06 → 06-06.

## Context

- Канон истории: `docs/story-canon.md` — целиком перед каждой правкой прозы.
- Карта сцен: `docs/story-scene-map.md` — запись юнита = обязательный вход
  задачи; бюджет-заметки модулей — там же.
- Гейты сцен: `.claude/skills/lecture-writer/references/checklist.md`.
  Скилл и линтер лежат в гитигноренном `.claude/` — артефакт локальной
  рабочей копии, в свежем клоне репозитория их нет: волны запускаются
  из этой рабочей копии.
- Модули волны: `lectures/04-querying-across-tables/`,
  `lectures/05-transactions-and-mvcc/`, `lectures/06-indexing-and-explain/` —
  правятся только `i18n/{ru,en}/README.md`.
- Предусловия окружения: команды из корня; `pnpm install` выполнен;
  Docker-песочница не нужна.

## Development Approach

- Одна задача = один юнит (RU+EN вместе), полностью до перехода дальше;
  внутри волны — по номерам.
- Правки — только `i18n/{ru,en}/README.md`. Код, `query.sql`, `sqlc.yaml`,
  `Makefile`, секции `## Запуск` / `## Running it` неприкасаемы.
- Полные сцены — только у ★ по карте; остальным — форма из записи карты.
  Сцена заменяет прозу (cap ×1.5) и обязана быть сильнее заменяемого
  открытия (правило отката). Модуль 04 уплотнён Тиер-0-проходом — сцены там
  только по правилу отката, существующие сильные открытия не трогать.
- Гейты зелёные — до следующей задачи. CRITICAL: при отклонении объёма
  обновлять план (➕/⚠️).
- Коммит на задачу: `docs:` — EN subject, RU body (формат по CLAUDE.md).

## Testing Strategy

- После каждого юнита — «линтер юнита» (команда в Technical Details).
- `make web-check-coverage` / `make web-build` — на чеке волны.
- Консистенси-чек волны: греп нитей §7, сверка голосов §2, полный линтер по
  трём модулям.

## Progress Tracking

- Отмечать `[x]` сразу; новые пункты — ➕, блокеры — ⚠️.
- При расхождении с планом — править план в том же коммите.

## Technical Details

- «Линтер юнита» = `node .claude/skills/lecture-writer/scripts/check_unit.mjs
  <путь-юнита> --against-head` (из корня; путь — в блоке Files задачи).
- Порядок работ: канон целиком → запись карты → текущие README → RU → EN →
  линтер → diff-скоуп.
- Модуль 05 — «неделя инцидентов»: плотность сцен выше (три ★ на модуль 05–06),
  но каждая сцена несёт типизированную функцию — «просто драма» не принимается.
- Персонажи данных (бариста Алиса/Борис, Карина) реплик не получают — в 05-04
  внутренний довод бариста остаётся прозой-пересказом.
- Открытый вопрос №3 (плашка-сводка: проза или `[!NOTE]`) может закрыться на
  06-01, если его микро-диалог деградирует в плашку, — тогда зафиксировать
  решение в разделе «Открытые вопросы» карты сцен.

## Implementation Steps

### Task 1: юнит `04-02-multi-table-and-self-joins` — микро-реплика

**Files:**
- Modify: `lectures/04-querying-across-tables/04-02-multi-table-and-self-joins/i18n/ru/README.md`
- Modify: `lectures/04-querying-across-tables/04-02-multi-table-and-self-joins/i18n/en/README.md`

Особое: камео и staff-Анна — разные фигуры (склейка §10.2 расторгнута):
камео — управляющий Руслан, а Анна из таблицы `staff` остаётся только
персонажем данных; при первом соседстве прозы с данными `staff` не склеивать
их и не давать Анне реплик; имена в демо/stdout не меняются.

- [x] прочитать канон целиком, запись `04-02` в карте сцен, оба README юнита
- [x] RU: микро-реплика по записи карты; гейты — чек-лист скилла lecture-writer
- [x] EN: зеркало — число реплик именованных персонажей равно RU, без he/she
  о читателе
- [x] линтер юнита — зелёный; diff юнита — только два README

### Task 2: юнит `04-03-aggregation-group-by-having` — микро-диалог

**Files:**
- Modify: `lectures/04-querying-across-tables/04-03-aggregation-group-by-having/i18n/ru/README.md`
- Modify: `lectures/04-querying-across-tables/04-03-aggregation-group-by-having/i18n/en/README.md`

- [x] прочитать канон целиком, запись `04-03` в карте сцен, оба README юнита
- [x] RU: микро-диалог по записи карты
- [x] EN: зеркало — число реплик именованных персонажей равно RU
- [x] линтер юнита — зелёный; diff юнита — только два README

### Task 3: юнит `04-04-distinct-on` — микро-реплика

**Files:**
- Modify: `lectures/04-querying-across-tables/04-04-distinct-on/i18n/ru/README.md`
- Modify: `lectures/04-querying-across-tables/04-04-distinct-on/i18n/en/README.md`

- [x] прочитать канон целиком, запись `04-04` в карте сцен, оба README юнита
- [x] RU: микро-реплика по записи карты
- [x] EN: зеркало — число реплик именованных персонажей равно RU
- [x] линтер юнита — зелёный; diff юнита — только два README

### Task 4: юнит `04-05-subqueries-exists-vs-in` — микро-диалог

**Files:**
- Modify: `lectures/04-querying-across-tables/04-05-subqueries-exists-vs-in/i18n/ru/README.md`
- Modify: `lectures/04-querying-across-tables/04-05-subqueries-exists-vs-in/i18n/en/README.md`

Особое: эпизод «сериала Карины» (реестр §7) — Карина сама реплик не получает.

- [x] прочитать канон целиком, запись `04-05` в карте сцен, оба README юнита
- [x] RU: микро-диалог по записи карты
- [x] EN: зеркало — число реплик именованных персонажей равно RU
- [x] линтер юнита — зелёный; diff юнита — только два README

### Task 5: юнит `04-06-ctes-and-materialization` — ★ полная сцена

**Files:**
- Modify: `lectures/04-querying-across-tables/04-06-ctes-and-materialization/i18n/ru/README.md`
- Modify: `lectures/04-querying-across-tables/04-06-ctes-and-materialization/i18n/en/README.md`

Особое: образец перекура — §9.5 канона (пример писан для 04-06).

- [ ] прочитать канон целиком, запись `04-06` в карте сцен, оба README юнита
- [ ] RU: сцена по записи карты (cap ×1.5, сцена заменяет прозу боли)
- [ ] EN: зеркало — число реплик именованных персонажей равно RU
- [ ] линтер юнита — зелёный; diff юнита — только два README

### Task 6: юнит `05-01-transactions-and-acid` — микро-диалог

**Files:**
- Modify: `lectures/05-transactions-and-mvcc/05-01-transactions-and-acid/i18n/ru/README.md`
- Modify: `lectures/05-transactions-and-mvcc/05-01-transactions-and-acid/i18n/en/README.md`

- [ ] прочитать канон целиком, запись `05-01` в карте сцен, оба README юнита
- [ ] RU: микро-диалог по записи карты
- [ ] EN: зеркало — число реплик именованных персонажей равно RU
- [ ] линтер юнита — зелёный; diff юнита — только два README

### Task 7: юнит `05-02-mvcc-mental-model` — шапка Павла над Заборчиком

**Files:**
- Modify: `lectures/05-transactions-and-mvcc/05-02-mvcc-mental-model/i18n/ru/README.md`
- Modify: `lectures/05-transactions-and-mvcc/05-02-mvcc-mental-model/i18n/en/README.md`

Особое: вторая (последняя) «шапка Павла» курса — ритм по первому носителю 02-03
и §4.4 (чат-жанр строчными).

- [ ] прочитать канон целиком, запись `05-02` в карте сцен, оба README юнита
- [ ] RU: шапка Павла по записи карты
- [ ] EN: зеркало — число реплик именованных персонажей равно RU
- [ ] линтер юнита — зелёный; diff юнита — только два README

### Task 8: юнит `05-03-row-locks-and-lost-updates` — микро-реплика

**Files:**
- Modify: `lectures/05-transactions-and-mvcc/05-03-row-locks-and-lost-updates/i18n/ru/README.md`
- Modify: `lectures/05-transactions-and-mvcc/05-03-row-locks-and-lost-updates/i18n/en/README.md`

- [ ] прочитать канон целиком, запись `05-03` в карте сцен, оба README юнита
- [ ] RU: микро-реплика по записи карты
- [ ] EN: зеркало — число реплик именованных персонажей равно RU
- [ ] линтер юнита — зелёный; diff юнита — только два README

### Task 9: юнит `05-04-isolation-levels-for-devs` — микро-реплика

**Files:**
- Modify: `lectures/05-transactions-and-mvcc/05-04-isolation-levels-for-devs/i18n/ru/README.md`
- Modify: `lectures/05-transactions-and-mvcc/05-04-isolation-levels-for-devs/i18n/en/README.md`

Особое: лучшая боль-проза модуля (write-skew) НЕ заменяется — рапорт Руслана
добавляет лицо; довод бариста остаётся прозой (персонажи данных без реплик);
перекур сеет цену ретраев — payoff в 05-05.

- [ ] прочитать канон целиком, запись `05-04` в карте сцен, оба README юнита
- [ ] RU: микро-реплика + перекур по записи карты
- [ ] EN: зеркало — число реплик именованных персонажей равно RU
- [ ] линтер юнита — зелёный; diff юнита — только два README

### Task 10: юнит `05-05-retry-on-40001` — ★ полная сцена

**Files:**
- Modify: `lectures/05-transactions-and-mvcc/05-05-retry-on-40001/i18n/ru/README.md`
- Modify: `lectures/05-transactions-and-mvcc/05-05-retry-on-40001/i18n/en/README.md`

Особое: тикет Ботыра «база нестабильна» — нормализация ошибки, Ботыр не
громоотвод; SQLSTATE и числа stdout в реплики не тащить.

- [ ] прочитать канон целиком, запись `05-05` в карте сцен, оба README юнита
- [ ] RU: сцена по записи карты (cap ×1.5, сцена заменяет прозу реакции)
- [ ] EN: зеркало — число реплик именованных персонажей равно RU
- [ ] линтер юнита — зелёный; diff юнита — только два README

### Task 11: юнит `05-06-deadlocks-and-advisory-locks` — ★ полная сцена

**Files:**
- Modify: `lectures/05-transactions-and-mvcc/05-06-deadlocks-and-advisory-locks/i18n/ru/README.md`
- Modify: `lectures/05-transactions-and-mvcc/05-06-deadlocks-and-advisory-locks/i18n/en/README.md`

Особое: финал модуля 05 — постмортем «виноват порядок, не человек» (callback
к 02-06); мостик-реплика Павла в модуль 06 — дословно по §6 канона; запись в
«блокнот Павла» (нить §7).

- [ ] прочитать канон целиком, запись `05-06` в карте сцен, оба README юнита
- [ ] RU: сцена по записи карты (cap ×1.5)
- [ ] EN: зеркало — число реплик именованных персонажей равно RU
- [ ] линтер юнита — зелёный; diff юнита — только два README

### Task 12: юнит `06-01-reading-explain-analyze-buffers` — микро-диалог

**Files:**
- Modify: `lectures/06-indexing-and-explain/06-01-reading-explain-analyze-buffers/i18n/ru/README.md`
- Modify: `lectures/06-indexing-and-explain/06-01-reading-explain-analyze-buffers/i18n/en/README.md`

Особое: рапорт Руслана гармонизировать с канонной болью юнита (админка, вечер) —
образец плашки §9.4 иллюстрирует форму, не факты; катехизис-рефрен «что база
ответила» — по реестру §7; при деградации в плашку — зафиксировать решение
вопроса №3 в карте сцен.

- [ ] прочитать канон целиком, запись `06-01` в карте сцен, оба README юнита
- [ ] RU: микро-диалог по записи карты
- [ ] EN: зеркало — число реплик именованных персонажей равно RU
- [ ] линтер юнита — зелёный; diff юнита — только два README

### Task 13: юнит `06-02-btree-and-composite-column-order` — микро-реплика

**Files:**
- Modify: `lectures/06-indexing-and-explain/06-02-btree-and-composite-column-order/i18n/ru/README.md`
- Modify: `lectures/06-indexing-and-explain/06-02-btree-and-composite-column-order/i18n/en/README.md`

- [ ] прочитать канон целиком, запись `06-02` в карте сцен, оба README юнита
- [ ] RU: микро-реплика + перекур по записи карты
- [ ] EN: зеркало — число реплик именованных персонажей равно RU
- [ ] линтер юнита — зелёный; diff юнита — только два README

### Task 14: юнит `06-03-when-indexes-dont-help` — микро-реплика

**Files:**
- Modify: `lectures/06-indexing-and-explain/06-03-when-indexes-dont-help/i18n/ru/README.md`
- Modify: `lectures/06-indexing-and-explain/06-03-when-indexes-dont-help/i18n/en/README.md`

- [ ] прочитать канон целиком, запись `06-03` в карте сцен, оба README юнита
- [ ] RU: микро-реплика по записи карты
- [ ] EN: зеркало — число реплик именованных персонажей равно RU
- [ ] линтер юнита — зелёный; diff юнита — только два README

### Task 15: юнит `06-04-partial-covering-and-unique` — микро-реплика

**Files:**
- Modify: `lectures/06-indexing-and-explain/06-04-partial-covering-and-unique/i18n/ru/README.md`
- Modify: `lectures/06-indexing-and-explain/06-04-partial-covering-and-unique/i18n/en/README.md`

- [ ] прочитать канон целиком, запись `06-04` в карте сцен, оба README юнита
- [ ] RU: микро-реплика + перекур по записи карты
- [ ] EN: зеркало — число реплик именованных персонажей равно RU
- [ ] линтер юнита — зелёный; diff юнита — только два README

### Task 16: юнит `06-05-gin-for-jsonb-and-arrays` — микро-реплика

**Files:**
- Modify: `lectures/06-indexing-and-explain/06-05-gin-for-jsonb-and-arrays/i18n/ru/README.md`
- Modify: `lectures/06-indexing-and-explain/06-05-gin-for-jsonb-and-arrays/i18n/en/README.md`

- [ ] прочитать канон целиком, запись `06-05` в карте сцен, оба README юнита
- [ ] RU: микро-реплика по записи карты
- [ ] EN: зеркало — число реплик именованных персонажей равно RU
- [ ] линтер юнита — зелёный; diff юнита — только два README

### Task 17: юнит `06-06-create-index-concurrently` — ★ полная сцена

**Files:**
- Modify: `lectures/06-indexing-and-explain/06-06-create-index-concurrently/i18n/ru/README.md`
- Modify: `lectures/06-indexing-and-explain/06-06-create-index-concurrently/i18n/en/README.md`

Особое: кульминация-рифма с 02-06 (payoff нити «Это было один раз/два раза»
и рифма постмортемов); блокнот Павла открыт на странице 02-06.

- [ ] прочитать канон целиком, запись `06-06` в карте сцен, оба README юнита
- [ ] RU: сцена по записи карты (cap ×1.5)
- [ ] EN: зеркало — число реплик именованных персонажей равно RU
- [ ] линтер юнита — зелёный; diff юнита — только два README

### Task 18: Verify acceptance criteria (консистенси-чек волны II)

**Files:**
- Modify: `docs/story-canon.md` (пометки статуса нитей §7)

- [ ] полный линтер волны: `node
  .claude/skills/lecture-writer/scripts/check_unit.mjs
  lectures/04-querying-across-tables lectures/05-transactions-and-mvcc
  lectures/06-indexing-and-explain --against-head` — ноль ошибок,
  предупреждения разобраны
- [ ] нити §7: греп по греп-маркерам (RU и EN) для нитей волны; список
  «setup написан, payoff ещё нет» отметить в реестре §7 канона
- [ ] сверка голосов: все сцены волны подряд «глазами» каждого говорившего
  персонажа против реестра §2; отклонения поправить
- [ ] `make web-check-coverage` и `make web-build` — зелёные
- [ ] скоуп прогона: каждый коммит прогона (все они правят этот план-файл;
  список — `git log --format=%h --
  docs/plans/20260704-story-uplift-2-wave-ii.md`) под `lectures/` меняет
  только `i18n/{ru,en}/README.md` юнитов волны (проверить `git show --stat`)
- [ ] все требования Overview выполнены (17 юнитов, ключевые биты волны на
  месте)

## Post-Completion

*Ручные шаги владельца — без чекбоксов.*

- Приёмка волны: выборочное чтение (минимум все ★, шапка Павла 05-02, рифма
  06-06), RU и EN.
- Дальше: `ralphex docs/plans/20260704-story-uplift-3-wave-iii.md`.
