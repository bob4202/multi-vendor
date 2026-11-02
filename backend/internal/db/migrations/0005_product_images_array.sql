-- +goose Up

-- A product can have more than one photo (storefront carousel), so the
-- single image_url column becomes an ordered list. Existing single URLs are
-- preserved by wrapping them in a one-element array rather than dropped.
ALTER TABLE product_content RENAME COLUMN image_url TO image_urls;
ALTER TABLE product_content
    ALTER COLUMN image_urls TYPE TEXT[]
    USING (CASE WHEN image_urls IS NULL THEN NULL ELSE ARRAY[image_urls] END);

ALTER TABLE branch_product_content RENAME COLUMN image_url_override TO image_urls_override;
ALTER TABLE branch_product_content
    ALTER COLUMN image_urls_override TYPE TEXT[]
    USING (CASE WHEN image_urls_override IS NULL THEN NULL ELSE ARRAY[image_urls_override] END);

-- +goose Down

ALTER TABLE branch_product_content
    ALTER COLUMN image_urls_override TYPE TEXT
    USING (image_urls_override[1]);
ALTER TABLE branch_product_content RENAME COLUMN image_urls_override TO image_url_override;

ALTER TABLE product_content
    ALTER COLUMN image_urls TYPE TEXT
    USING (image_urls[1]);
ALTER TABLE product_content RENAME COLUMN image_urls TO image_url;
