-- +goose Up
CREATE TABLE categories (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL CHECK (char_length(trim(name)) BETWEEN 1 AND 80),
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX categories_name_unique_idx ON categories (lower(trim(name)));

CREATE TABLE products (
    id UUID PRIMARY KEY,
    category_id UUID NOT NULL REFERENCES categories(id),
    name TEXT NOT NULL CHECK (char_length(trim(name)) BETWEEN 1 AND 120),
    product_type TEXT NOT NULL CHECK (product_type IN ('ingredient', 'produced', 'resale')),
    sale_mode TEXT NOT NULL CHECK (sale_mode IN ('unit', 'weight')),
    base_unit TEXT NOT NULL CHECK (base_unit IN ('unit', 'g', 'ml')),
    sellable BOOLEAN NOT NULL DEFAULT TRUE,
    track_lots BOOLEAN NOT NULL DEFAULT FALSE,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (
        (sale_mode = 'unit' AND base_unit = 'unit') OR
        (sale_mode = 'weight' AND base_unit = 'g') OR
        (sellable = FALSE)
    )
);

CREATE UNIQUE INDEX products_name_unique_idx ON products (lower(trim(name)));
CREATE INDEX products_category_idx ON products(category_id, active, name);
CREATE INDEX products_active_name_idx ON products(active, name);

CREATE TABLE product_barcodes (
    id UUID PRIMARY KEY,
    product_id UUID NOT NULL UNIQUE REFERENCES products(id),
    code TEXT NOT NULL CHECK (char_length(trim(code)) BETWEEN 1 AND 64),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (product_id, code),
    UNIQUE (code)
);

CREATE TABLE product_prices (
    id UUID PRIMARY KEY,
    product_id UUID NOT NULL REFERENCES products(id),
    amount_cents BIGINT NOT NULL CHECK (amount_cents > 0),
    price_unit TEXT NOT NULL CHECK (price_unit IN ('unit', 'kg')),
    effective_from TIMESTAMPTZ NOT NULL,
    effective_to TIMESTAMPTZ,
    actor_id UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (effective_to IS NULL OR effective_to >= effective_from)
);

CREATE UNIQUE INDEX product_prices_current_unique_idx
    ON product_prices(product_id) WHERE effective_to IS NULL;
CREATE INDEX product_prices_history_idx ON product_prices(product_id, effective_from DESC);

CREATE TABLE price_change_batches (
    id UUID PRIMARY KEY,
    kind TEXT NOT NULL CHECK (kind IN ('manual', 'bulk', 'import')),
    actor_id UUID NOT NULL REFERENCES users(id),
    request_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE price_change_items (
    batch_id UUID NOT NULL REFERENCES price_change_batches(id),
    product_id UUID NOT NULL REFERENCES products(id),
    old_amount_cents BIGINT,
    new_amount_cents BIGINT NOT NULL CHECK (new_amount_cents > 0),
    PRIMARY KEY (batch_id, product_id)
);

CREATE TABLE catalog_state (
    singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
    version BIGINT NOT NULL CHECK (version > 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO catalog_state (singleton, version) VALUES (TRUE, 1);

CREATE TABLE inventory_balances (
    product_id UUID PRIMARY KEY REFERENCES products(id),
    quantity NUMERIC(18,3) NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE inventory_movements (
    id UUID PRIMARY KEY,
    product_id UUID NOT NULL REFERENCES products(id),
    quantity_delta NUMERIC(18,3) NOT NULL CHECK (quantity_delta <> 0),
    movement_type TEXT NOT NULL CHECK (
        movement_type IN ('initial', 'purchase', 'production', 'sale', 'return', 'waste', 'adjustment')
    ),
    reference_type TEXT NOT NULL,
    reference_id UUID,
    actor_id UUID NOT NULL REFERENCES users(id),
    request_id TEXT NOT NULL,
    reason TEXT,
    occurred_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX inventory_movements_product_idx
    ON inventory_movements(product_id, occurred_at DESC, id);

-- +goose Down
DROP TABLE inventory_movements;
DROP TABLE inventory_balances;
DROP TABLE catalog_state;
DROP TABLE price_change_items;
DROP TABLE price_change_batches;
DROP TABLE product_prices;
DROP TABLE product_barcodes;
DROP TABLE products;
DROP TABLE categories;
