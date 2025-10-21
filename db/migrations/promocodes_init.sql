CREATE TABLE promo_codes (
    id SERIAL PRIMARY KEY,
    brand VARCHAR(20) NOT NULL CHECK (brand IN ('JET', 'YANDEX', 'WHOOSH', 'BOLT')),
    promo_code VARCHAR(100) NOT NULL,
    valid_until DATE NOT NULL,          -- до какого числа можно выдать
    assigned_to_user_id INT DEFAULT NULL,
    claimed_at TIMESTAMPTZ DEFAULT NULL,
    created_by_admin_id INT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_promo_brand_valid ON promo_codes (brand, valid_until);
CREATE INDEX idx_promo_unclaimed ON promo_codes (brand, valid_until, assigned_to_user_id) 
    WHERE assigned_to_user_id IS NULL;