package messaging

// RegisteredMsgTags exposes the codec's registered wire tags so the
// registration-coverage audit can verify every message type in the module is
// decodable from a checkpoint.
func RegisteredMsgTags() []string {
	return msgCodec.Tags()
}
