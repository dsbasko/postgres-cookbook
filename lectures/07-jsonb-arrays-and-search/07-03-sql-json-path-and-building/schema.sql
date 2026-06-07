-- schema.sql — DDL-добавки ЭТОГО юнита поверх канона Brew (schema/brew.sql).
--
-- Тема — SQL/JSON path и сборка jsonb. Нужны вложенные документы (массив
-- ингредиентов с граммами) — заводим СВОЙ лабораторный стол drink_recipe_lab,
-- чтобы свободно ставить опыты с путями и не трогать канон. Сборку (jsonb_agg)
-- показываем уже на каноне drinks.
--
-- DDL применяется двумя путями: sqlc читает его на `make gen`, а демо встраивает
-- через runtime.Caller и накатывает brew.Apply при db-reset. Поэтому он
-- идемпотентен: DROP TABLE IF EXISTS + CREATE + детерминированный seed.

DROP TABLE IF EXISTS drink_recipe_lab;

-- drink_recipe_lab — рецепт напитка как jsonb: калории и массив ингредиентов
-- (имя + граммы). Вложенный массив объектов — ровно тот случай, где блистает
-- jsonpath: достать поля по выражению пути и отфильтровать элементы по условию.
CREATE TABLE drink_recipe_lab (
    id     bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name   text  NOT NULL,
    recipe jsonb NOT NULL
);

INSERT INTO drink_recipe_lab (name, recipe) VALUES
    ('Латте',    '{"kcal":190,"ingredients":[{"name":"espresso","grams":30},{"name":"milk","grams":220}]}'),
    ('Эспрессо', '{"kcal":3,"ingredients":[{"name":"espresso","grams":30}]}'),
    ('Колд брю', '{"kcal":5,"ingredients":[{"name":"coffee","grams":60},{"name":"water","grams":300}]}');
