-- schema.sql — DDL-добавки ЭТОГО юнита поверх канона Brew (schema/brew.sql).
--
-- Свой тип enum (drink_size) — на нём показываем, что enum упорядочен по порядку
-- ОБЪЯВЛЕНИЯ значений, а не по алфавиту. Массивы и jsonb своих таблиц не требуют:
-- массивы собираем из articles.tags (string_to_array), jsonb — из литералов.
--
-- Применяется двумя путями: sqlc читает на `make gen` (чтобы знать тип enum), а
-- демо встраивает через runtime.Caller и накатывает brew.Apply при db-reset.
--
-- Идемпотентность: CREATE TYPE не поддерживает IF NOT EXISTS, поэтому сначала
-- DROP TYPE IF EXISTS. CASCADE безопасен — ни одна таблица на этот тип не
-- завязана (демо использует его только в литералах вида 'small'::drink_size).
DROP TYPE IF EXISTS drink_size CASCADE;
CREATE TYPE drink_size AS ENUM ('small', 'medium', 'large');
