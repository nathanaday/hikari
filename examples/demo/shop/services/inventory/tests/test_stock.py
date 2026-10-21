import unittest

from inventory import Stock


class StockTest(unittest.TestCase):
    def test_reserve_holds_stock(self):
        stock = Stock({"apple": 3})
        self.assertTrue(stock.reserve("apple", 2))
        self.assertEqual(stock.available("apple"), 1)

    def test_reserve_fails_when_too_little_is_available(self):
        stock = Stock({"apple": 1})
        self.assertFalse(stock.reserve("apple", 2))
        self.assertEqual(stock.available("apple"), 1)

    def test_restock_adds_stock(self):
        stock = Stock()
        stock.restock("pear", 5)
        self.assertEqual(stock.available("pear"), 5)


if __name__ == "__main__":
    unittest.main()
