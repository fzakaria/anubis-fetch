;; Exercise cancellation without depending on the cost of a real challenge.
(module
  (memory (export "memory") 1)
  (func (export "data_ptr") (result i32) i32.const 0)
  (func (export "set_data_length") (param i32))
  (func (export "anubis_work") (param i32 i32 i32) (result i32)
    (loop $forever br $forever)
    i32.const 0)
  (func (export "result_hash_ptr") (result i32) i32.const 0)
  (func (export "result_hash_size") (result i32) i32.const 32))
