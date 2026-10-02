-- Фотографии объекта: миниатюра для сетки превью и размеры исходника.
-- Файлы лежат в подкаталоге объекта: data/photos/<inventory_id>/<uuid>.<ext>,
-- миниатюры — рядом, в data/photos/<inventory_id>/thumbs/<uuid>.jpg.
ALTER TABLE inventory_photos ADD COLUMN thumb_name TEXT NOT NULL DEFAULT '';
ALTER TABLE inventory_photos ADD COLUMN width INTEGER NOT NULL DEFAULT 0;
ALTER TABLE inventory_photos ADD COLUMN height INTEGER NOT NULL DEFAULT 0;
