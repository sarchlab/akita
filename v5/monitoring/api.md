# AkitaRTM API Documentation

## Individual API

### Buffer Levels

The buffer levels API returns the buffer information for the most occupied buffers. Here, we refer `level` as the number of items in the buffer, and `cap` as the maximum number of items that can be stored in the buffer. The `percent` field is the percentage of the buffer that is occupied.

**Endpoint:** `GET /api/individual/buffer_levels`

**Parameters:**

These parameters are GET parameters.

* Sort: `level` or `percent`. Default is `level`.
* Limit: The maximum number of buffers to return. Value 0 means no limit. Default is 0.
* Offset: The offset of the first buffer to return. Default is 0.

**Response:**

```json
{
  "data": [
	{
	  "buffer": "buffer1",
	  "level": 10,
	  "cap": 100
	},
	{
	  "buffer": "buffer2",
	  "level": 20,
	  "cap": 100
	}
  ]
}
```





