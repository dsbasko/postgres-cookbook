-- query.sql — протагонист урока. `make gen` → типизированный pgx-код в
-- internal/db/. Имя после `-- name:` → метод; суффикс — форма результата.
--
-- Тема — нечёткий поиск pg_trgm. similarity(a, b) меряет, насколько похожи строки
-- по общим триграммам (0..1); оператор % истинен, когда схожесть выше порога
-- pg_trgm.similarity_threshold (по умолчанию 0.3). Это ловит ОПЕЧАТКИ — то, чего
-- ни LIKE, ни полнотекстовый поиск (07-05) не умеют. Запрос здесь — 'capucino'
-- (опечатка в 'Cappuccino': пропущены p и c).

-- name: SimilarityScores :many
-- similarity к каждому пункту меню, по убыванию. У 'Cappuccino' схожесть высокая,
-- у остальных — почти ноль: триграммы 'capucino' совпадают в основном с ним.
-- round(...)::text — стабильное печатаемое число (округляем real до 3 знаков).
SELECT name, round(similarity(name, 'capucino')::numeric, 3)::text AS sim
FROM menu_search_lab
ORDER BY similarity(name, 'capucino') DESC, id;

-- name: DidYouMean :many
-- Оператор % оставляет только то, что выше порога схожести (0.3) — готовый
-- «возможно, вы имели в виду». На опечатку 'capucino' проходит лишь 'Cappuccino'.
SELECT name, round(similarity(name, 'capucino')::numeric, 3)::text AS sim
FROM menu_search_lab
WHERE name % 'capucino'
ORDER BY similarity(name, 'capucino') DESC, id;

-- name: AcceleratedLike :many
-- Бонус trgm-индекса: ILIKE '%подстрока%' с шаблоном в середине обычный B-tree
-- не ускоряет, а GIN gin_trgm_ops — да (06-05 про индексы). Семантика та же, что
-- у обычного ILIKE; ускорение проявляется на большой таблице.
SELECT name
FROM menu_search_lab
WHERE name ILIKE '%presso%'
ORDER BY id;
