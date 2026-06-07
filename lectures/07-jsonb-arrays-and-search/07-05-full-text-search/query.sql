-- query.sql — протагонист урока. `make gen` → типизированный pgx-код в
-- internal/db/. Имя после `-- name:` → метод; суффикс — форма результата.
--
-- Тема — полнотекстовый поиск: текст превращается в tsvector (нормализованные
-- лексемы с позициями), запрос — в tsquery, оператор @@ проверяет совпадение, а
-- ts_rank ранжирует. В отличие от LIKE '%brew%', FTS понимает морфологию (brewing
-- → brew) и игнорирует стоп-слова, а с GIN по tsv летит индексом (06-05).

-- name: ShowTsvector :one
-- Что видит поиск: тело статьи 2, разобранное в tsvector. Заметь стемминг
-- (brewing/brew → 'brew', temperature → 'temperatur', hours → 'hour') и
-- выброшенные стоп-слова (is, about, not, for). Числа — позиции лексем в тексте.
SELECT to_tsvector('english', body)::text AS tsv
FROM kb_articles
WHERE id = 2;

-- name: SearchRanked :many
-- Поиск 'brew' с ранжированием. @@ — оператор совпадения tsvector ↔ tsquery.
-- ts_rank учитывает веса: у статьи 2 'brew' есть и в заголовке (вес A), и в теле
-- (B), поэтому её ранг выше, чем у статьи 1 (только тело). round(...)::text —
-- стабильное печатаемое число (float ранга округляем до 4 знаков).
SELECT id, title,
       round(ts_rank(tsv, plainto_tsquery('english', 'brew'))::numeric, 4)::text AS rank
FROM kb_articles
WHERE tsv @@ plainto_tsquery('english', 'brew')
ORDER BY rank DESC, id;

-- name: SearchAnd :many
-- to_tsquery с оператором & — обе лексемы должны присутствовать. Находим статьи,
-- где есть И milk, И cappuccino (plainto_tsquery соединил бы их тоже через &, но
-- to_tsquery даёт явный контроль: |, &, !, <-> для фразового поиска).
SELECT id, title
FROM kb_articles
WHERE tsv @@ to_tsquery('english', 'milk & cappuccino')
ORDER BY id;

-- name: StemmingMatch :many
-- Морфология бесплатно: запрос 'brewing' стеммится в 'brew' и находит статьи,
-- где встречается 'brew'/'brewing' — то, чего LIKE '%brewing%' не дал бы.
SELECT id, title
FROM kb_articles
WHERE tsv @@ plainto_tsquery('english', 'brewing')
ORDER BY id;
