-- demo.sql — рекурсивные CTE: обход дерева и защита от циклов (цель `make run`).
--
-- Escape-hatch-юнит (как 05-02/06-01): sqlc v1.30.0 НЕ понимает SQL-стандартную
-- секцию CYCLE (она вводит «виртуальные» колонки is_cycle/path, которых нет в
-- схеме, — sqlc падает с «column is_cycle does not exist»). А урок ровно про неё.
-- Поэтому выбираем фичу, а не инструмент: ведём демо psql-скриптом.
--
-- Две части: (1) WITH RECURSIVE спускается по дереву категорий меню Brew, считая
-- глубину и путь; (2) на нарочно зациклённом графе маршрутов показываем, как
-- секция CYCLE ловит повтор и ОСТАНАВЛИВАЕТ рекурсию (без неё — бесконечный цикл).
--
-- Детерминизм: сортируем по МАССИВУ id (idpath), а не по строке имени — порядок
-- не зависит от collation базы. Лабораторные столы создаются и дропаются здесь
-- же, канон Brew не трогаем, вывод воспроизводится дословно при любом прогоне.

\set ON_ERROR_STOP on
\pset footer off
SET client_min_messages = warning;   -- глушим NOTICE от DROP ... IF EXISTS

-- ── Часть 1. Дерево категорий меню Brew ───────────────────────────────────
DROP TABLE IF EXISTS category_tree_lab;
CREATE TABLE category_tree_lab (
    id        bigint PRIMARY KEY,
    parent_id bigint REFERENCES category_tree_lab (id),
    name      text   NOT NULL
);
INSERT INTO category_tree_lab (id, parent_id, name) VALUES
    (1, NULL, 'Напитки'),
    (2, 1,    'Кофе'),
    (3, 1,    'Чай'),
    (4, 2,    'Эспрессо-напитки'),
    (5, 2,    'Фильтр'),
    (6, 4,    'Капучино'),
    (7, 4,    'Латте'),
    (8, 3,    'Зелёный');

\echo '1) Обход дерева категорий сверху вниз (WITH RECURSIVE):'
WITH RECURSIVE tree AS (
    -- якорь: корни (нет родителя), глубина 1, путь = [свой id]
    SELECT id, parent_id, name, 1 AS depth,
           ARRAY[id]   AS idpath,
           name::text  AS namepath
    FROM category_tree_lab
    WHERE parent_id IS NULL
    UNION ALL
    -- рекурсивный шаг: дети уже найденных узлов, глубина +1, путь дополняем
    SELECT c.id, c.parent_id, c.name, t.depth + 1,
           t.idpath   || c.id,
           t.namepath || ' > ' || c.name
    FROM category_tree_lab c
    JOIN tree t ON c.parent_id = t.id
)
SELECT depth,
       repeat('  ', depth - 1) || name AS category,
       namepath                        AS path
FROM tree
ORDER BY idpath;   -- массив id → детерминированный pre-order, без зависимости от collation

-- ── Часть 2. Защита от цикла: секция CYCLE ────────────────────────────────
-- Маршруты «куда дальше»: Склад → Цех → Кафе → … → Склад (петля!). Без защиты
-- рекурсия по такому графу не завершится никогда.
DROP TABLE IF EXISTS cyclic_routes_lab;
CREATE TABLE cyclic_routes_lab (
    id      bigint PRIMARY KEY,
    next_id bigint NOT NULL,
    name    text   NOT NULL
);
INSERT INTO cyclic_routes_lab (id, next_id, name) VALUES
    (1, 2, 'Склад'),
    (2, 3, 'Цех'),
    (3, 1, 'Кафе');   -- ← замыкает петлю обратно на склад

\echo ''
\echo '2) Тот же обход по зациклённому графу, но с CYCLE — рекурсия сама тормозит:'
WITH RECURSIVE walk AS (
    SELECT id, next_id, name, 1 AS step
    FROM cyclic_routes_lab
    WHERE id = 1
    UNION ALL
    SELECT r.id, r.next_id, r.name, w.step + 1
    FROM cyclic_routes_lab r
    JOIN walk w ON r.id = w.next_id
) CYCLE id SET is_cycle USING path   -- помечаем повтор id и кладём пройденный путь в path
SELECT step, id, name, is_cycle, path
FROM walk
ORDER BY step;
