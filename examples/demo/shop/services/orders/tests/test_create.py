import unittest

from orders import OrderError, create_order


class CreateOrderTest(unittest.TestCase):
    def test_reserves_each_item(self):
        reserved = []
        order = create_order({"apple": 2, "pear": 1}, lambda sku, q: reserved.append((sku, q)) or True)
        self.assertEqual(order.status, "placed")
        self.assertEqual(reserved, [("apple", 2), ("pear", 1)])

    def test_rejects_an_empty_order(self):
        with self.assertRaises(OrderError):
            create_order({}, lambda sku, q: True)

    def test_rejects_a_bad_quantity(self):
        with self.assertRaises(OrderError):
            create_order({"apple": 0}, lambda sku, q: True)

    def test_fails_without_stock(self):
        with self.assertRaises(OrderError):
            create_order({"apple": 1}, lambda sku, q: False)


if __name__ == "__main__":
    unittest.main()
