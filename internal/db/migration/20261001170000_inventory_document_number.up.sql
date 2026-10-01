-- 20261001170000_inventory_document_number.up.sql
-- Опись собирается из документов заявок. Номер самого документа (105, 07,2)
-- храним отдельной колонкой, чтобы строка была привязана к своей заявке:
-- в наименовании он не читается, а в «Инв. №» лежит код номенклатуры.
ALTER TABLE inventory_numbers ADD COLUMN document_number TEXT NOT NULL DEFAULT '';
