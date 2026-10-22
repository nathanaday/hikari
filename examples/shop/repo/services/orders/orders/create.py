"""Order intake: check an order and reserve its stock."""

import itertools
from dataclasses import dataclass

_ids = itertools.count(1)


class OrderError(ValueError):
    pass


@dataclass
class Order:
    id: int
    items: dict  # sku -> quantity
    status: str = "placed"


def create_order(items, reserve):
    """Create an order for items, a dict of sku to quantity.

    reserve(sku, quantity) is the inventory's reserve interface. It returns
    True when the stock is held for the order.
    """
    if not items:
        raise OrderError("an order needs at least one item")
    for sku, quantity in items.items():
        if quantity <= 0:
            raise OrderError(f"the quantity of {sku} must be positive")
    for sku, quantity in items.items():
        if not reserve(sku, quantity):
            raise OrderError(f"not enough stock for {sku}")
    return Order(id=next(_ids), items=dict(items))
