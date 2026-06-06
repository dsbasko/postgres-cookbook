-- schema.sql — DDL-добавки ЭТОГО юнита поверх канона Brew (schema/brew.sql).
--
-- Тема — трезвая семантика NULL и ловушка NOT IN + NULL. Чтобы воспроизвести
-- ловушку на реальных данных, нужен список напитков с затесавшимся NULL —
-- держим его в своей лабораторной таблице unavailable (drink_id NULLABLE, иначе
-- NULL не вставить). Канон не трогаем.
--
-- DDL применяется двумя путями: sqlc читает его на `make gen`, а демо встраивает
-- через runtime.Caller и накатывает brew.Apply при db-reset. Поэтому он
-- идемпотентен (CREATE TABLE IF NOT EXISTS).

-- unavailable — «недоступные напитки» (id из drinks). drink_id NULLABLE
-- намеренно: в проде такой источник часто допускает NULL (внешний фид, LEFT
-- JOIN), и ровно один NULL ломает NOT IN. Это и демонстрируем.
CREATE TABLE IF NOT EXISTS unavailable (
    drink_id  BIGINT  NULL
);
