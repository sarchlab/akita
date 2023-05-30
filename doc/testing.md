# Testing

Testing is vital to a simulator design. With proper testing, a developer can guarantee the component under development behave as desired. Testing can eliminate lots of bugs in early stages before the simulator is used in real experiments.

In Akita, we use 3 different types of tests, including unit tests, integration tests, and acceptance tests. We will show how to use them in the following sections. But before we dive into how to write tests, we discuss the test driven deveopment, the process of writing tests.

## Test Driven Development

Test Driven Development (TDD) generally describes the software development practice of writing tests before writing production code. Citing Rob Martin's three laws of TDD, they are:

> 1. You are not allowed to write any production code unless it is to make a failing unit test pass.
> 1. You are not allowed to write any more of a unit test than is sufficient to fail; and compilation failures are failures.
> 1. You are not allowed to write any more production code than is sufficient to pass the one failing unit test.

Assuming all the tests work at the beginning and you want to add or change a feature in the code, you would first write or change a test, run the tests, and make sure the tests fail. It is important to run the tests that are expected to fail. If you expect some passing test to fail, it may be a hint of some even more serious problems in the code. Then, you write the production code and keep running the tests. When the test passes, you should have done with the development and you should move to the test code again.

If all the changes in both the test code and the production code are simple enough, you should be switching between the test code and the production code very fast. Usually, each cycle should be around one to five minutes. By doing this, you can guarantee that any error you made must happen in the code that you changed in the past one to five minutes. Also, a common case is that when someone else modified some other part of the code, your code may stop working. With proper unit testing, this would not happen, as any piece of code you wrote are protected with the tests and you know that is all the tests pass, your code behaves exactly as you want.

The statement above is the ideal case, there are still problems cannot be solved by testing. We loosely follow the TDD laws and try our best to improve the test coverage. So what code needs to be tested and what code is OK to leave not to be tested? A rule of thumb is that if the code "has logic", then you need to test it. For example, the code that emulates an instruction and changes the register state, the code that handles one message and generates some other messages is considered as "has logic" and need to be tested, while the code that glues components together by setting the connections is considered not to "have logic" and does not have to have tests.

## Unit Testing

Unit tests mainly target at the component of sub-component level. We use Ginkgo test framework to write our tests.

### Install Ginkgo

To install Ginkgo, run the following commands:

```bash
go get -u github.com/onsi/ginkgo/ginkgo  # installs the ginkgo CLI
go get -u github.com/onsi/gomega/...     # fetches the matcher library
```

### Bootstrap tests for a package

For any go package to test, you would need to provide a simple script that connects Ginkgo with the go test system. You can use the following command to generate the script:

```bash
ginkgo bootstrap
```

Then, you can try running `ginkgo` in your terminal and see the output. At this moment, you do not have any tests, so zero tests are executed and the test suite passes.

### Write a unit test

Suppose you have a go file called `adder.go` and has the following code:

```go
func add(a, b int) int {
    return a + b;
}
```

You can define another file for tests

```go
package test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = Describe("Adder", func() {
    It("should add", func() {
        Expect(add(2, 3)).To(Equal(5));
    })
})
```

### Mocking

Writing a unit test like above is easy, as the adder function has no dependency to other code. However, in real code, a struct usually depends on many other structs and you may need to assume that other parts of the code are working properly so that you can test your code. This assumption usually does not hold as the change in another piece of code may break your test easily. Also, the dependency struct may have dependencies by themselves and testing one struct may become a test for the whole simulator.

To solve this problem, we need to break down the dependency chain. The main principle here is called Dependency Inversion Principle. Using Go terminology to understand this principle, your struct should not depend on another struct but should only depend on interfaces. With this requirement, one struct may only talk to interfaces rather than other structs, allowing the freedom to replace the structs that follow the same interface.

Then, the question is that how a struct know which detail struct to use on each interface? We use dependency injection. For example, in this code:

```go
interface Printer{
    Print()
}

struct DefaultPrinter{
    // Implement Print
}

struct SpecialPrinter{
    // Implement Print
}

struct SampleComponent {
    p Printer
}

func NewSampleComponent(p Printer) *SampleComponent {
    return SampleComponent{p: p}
}
```

the `SampleComponent` depends on a printer to print some value. The `SampleComponent` only knows that there is a printer available, but it does not know what exact type of printer it has. We inject the concrete printer from the `NewSampleComponent` function. This `NewSampleComponent` function should be called by a `Builder` function or struct, which usually serves as a configuration for Akita. By doing this, the `Builder` defines how the `SampleComponent` can print values.

Other than the flexibility of configuration, we simplify how we write unit tests. For example, in Akita, a component needs to send messages out and schedule events. In this case, we need to provide a special version of the `Engine` and the `Port` for testing purposes. Suppose we want to have a component that forwards messages from the input port to the output port, we can write the test like this.

```go
package test

import (
    "github.com/golang/mock/gomock"
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "github.com/sarchlab/akita"
    "github.com/sarchlab/akita/mock_akita"
)

var _ = Describe("Forwarder", func() {
    var (
        mockCtrl   *gomock.Controller
        engine     *mock_akita.MockEngine
        inputPort  *mock_akita.MockPort
        outputPort *mock_akita.MockPort
        forwarder  *Forwarder
    )

    BeforeEach(func() {
        mockCtrl = gomock.NewController(GinkgoT());
        engine = mock_akita.NewMockEngine(mockCtrl)
        toCP = mock_akita.NewMockPort(mockCtrl)
        toMem = mock_akita.NewMockPort(mockCtrl)

        forwarder = NewForwarder()
        forwarder.InputPort = inputPort
        forwarder.OutputPort = outputPort
    })

    AfterEach(func() {
        mockCtrl.Finish()
    })

    It("should forward", func(){
        msg := NewSomeMsg();
        inputPort.EXPECT().Peek().Return(msg)
        inputPort.EXPECT().Retrieve(akita.VTimeInSec(6)).Return(msg)
        engine.EXPECT().Schedule(gomock.Any())
        outputPort.EXPECT().Send(msg).Return(nil)

        forwarder.Forward(6) // the argument is the current time;
    })
})

```

By doing this, we can fully isolate the communication from one component to the rest of the simulator. If there is an error in this test, we know that the component must not work properly, not anywhere else in the simulator.

A key step to make mocking objects work are creating mock objects. We use gomock framework to create those mock object, as gomock provides a uniform style using the mock objects.

Gomock provides a code generator that can generate mock objects. You need to first install mockgen with the command:

```bash
go install github.com/golang/mock/mockgen
```

Suppose we want to mock the `Port` interface, we can simply run:

```bash
mockgen github.com/sarchlab/akita Port > mock_akita/mock_port.go
```

## Integration Testing

Integration tests are slightly higher than unit tests and may test a few components to work together. For example, testing a cache system can always return the data that has been written is a typical case of an integration test.

Integration tests also use ginkgo framework. But it generally does not use mocking. It also does not usually verify the private fields value. It only cares about the system input and output are as desired.

## Acceptance Testing

Acceptance tests are end-to-end tests. It is tested as if a user is using the simulator. A typical example is to test if the GPU timing simulator can generate the correct emulation result, while the estimated time is not too far off-chart from the real GPU execution. Acceptance testing is usually performed with bash or Python script.