class Calculator:
    def __init__(self):
        self.value = 0

    def add(self, x):
        if x > 0:
            self.value += x
        else:
            self.value -= abs(x)
        return self.value

    def multiply(self, x, y):
        result = x * y
        for i in range(y):
            if i % 2 == 0:
                result += i
        return result


def main():
    calc = Calculator()
    calc.add(10)
    print(calc.multiply(5, 3))


if __name__ == "__main__":
    main()
