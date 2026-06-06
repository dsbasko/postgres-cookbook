-- query.sql — протагонист урока. `make gen` → типизированный pgx-код в
-- internal/db/. Имя после `-- name:` → метод; суффикс — форма результата.
--
-- Тема — две вещи: (1) JOIN тянется через сколько угодно таблиц, и (2) таблицу
-- можно соединить саму с собой (self-join). Чек заказа собираем из четырёх
-- таблиц канона; иерархию персонала — self-join'ом лабораторной staff.

-- name: OrderReceipt :many
-- Многотабличный JOIN: orders → customers → order_items → drinks. Каждый JOIN
-- добавляет по таблице через свой ключ. Получаем «чек»: по строке на позицию
-- заказа с именем клиента, названием напитка, количеством и суммой строки.
SELECT
    o.id                          AS order_id,
    c.name                        AS customer,
    d.name                        AS drink,
    oi.quantity                            AS quantity,
    oi.unit_price                          AS unit_price,
    (oi.quantity * oi.unit_price)::bigint  AS line_total
FROM orders o
JOIN customers c   ON c.id::text = o.customer_id
JOIN order_items oi ON oi.order_id = o.id
JOIN drinks d      ON d.id = oi.drink_id
ORDER BY o.id, oi.id;

-- name: TruncateStaff :exec
TRUNCATE staff;

-- name: SeedStaff :exec
-- Анна — старший менеджер (manager_id NULL); остальные подчинены ей. Анна стоит
-- первой строкой: FK manager_id ссылается на уже вставленную строку этой же
-- таблицы.
INSERT INTO staff (id, name, role, manager_id) VALUES
    (1, 'Анна', 'manager',    NULL),
    (2, 'Борис', 'barista',   1),
    (3, 'Вера', 'barista',    1),
    (4, 'Глеб', 'shift-lead', 1);

-- name: StaffWithManager :many
-- Self-join: одна и та же таблица staff участвует дважды под разными псевдонимами
-- — e (employee) и m (manager). Связь e.manager_id = m.id «разворачивает» ссылку
-- в имя руководителя. LEFT JOIN, потому что у Анны руководителя нет → manager
-- придёт NULL (а не выкинет её из списка).
SELECT e.name AS employee, e.role, m.name AS manager
FROM staff e
LEFT JOIN staff m ON m.id = e.manager_id
ORDER BY e.id;
