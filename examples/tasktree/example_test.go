package main

func Example() {
	main()
	// Output:
	// req_out @ Client.req_out
	//   req_in @ L1.req_in
	//     req_out @ L1.req_out
	//       req_in @ L2.req_in
	//         req_out @ L2.req_out
	//           req_in @ Memory.req_in
}
