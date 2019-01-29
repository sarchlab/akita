We specify the general coding guideline so that we can keep the coding style consistent. Consistent codes are usually more readable and more fun to work with. All the subprojects of Project Akita, if written in Go, should follow the same coding guideline.

1. We follow the go default coding style. Please run `go imports` whenever possible. Ideally, you should configure your editor to run `go imports` on save. 

1. Each line of code should have less than 80 characters (with exception). 

1. Try your best to keep each function less than 10 lines. 

1. Writing comments for code blocks is a big NO~NO~NO~. Whenever you feel you need a sentence of comment to make the code more readable, use a function instead.

1. Functions should generally have less than 6 input argument, including constructors. Functions that have no or one arguments are strongly encouraged. You should try your best to reduce the number of arguments for each function, as fewer arguments reduce the short-term memory burden.

1. When you have multiple arguments in the function and you cannot fit the function declaration in one line, use the following format.

```go
func (m *Module) SampleFunction(
    arg1 *ArgType1,
    arg2 *ArgType2,
) *ReturnType {
    ...
}
```