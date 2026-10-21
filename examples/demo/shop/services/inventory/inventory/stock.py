"""Stock levels and reservations."""


class Stock:
    def __init__(self, levels=None):
        self._levels = dict(levels or {})
        self._held = {}

    def available(self, sku):
        """Return the quantity of sku that is in stock and not reserved."""
        return self._levels.get(sku, 0) - self._held.get(sku, 0)

    def reserve(self, sku, quantity):
        """Hold quantity of sku for an order. Return False if too little is available."""
        if quantity <= 0:
            raise ValueError("quantity must be positive")
        if self.available(sku) < quantity:
            return False
        self._held[sku] = self._held.get(sku, 0) + quantity
        return True

    def restock(self, sku, quantity):
        """Add quantity of sku to the stock level."""
        if quantity <= 0:
            raise ValueError("quantity must be positive")
        self._levels[sku] = self._levels.get(sku, 0) + quantity
