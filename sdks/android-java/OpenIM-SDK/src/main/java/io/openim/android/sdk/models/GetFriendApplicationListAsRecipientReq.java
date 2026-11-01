package io.openim.android.sdk.models;

public class GetFriendApplicationListAsRecipientReq {
    private int offset;
    private int count;
    private int[] handleResults;

    public GetFriendApplicationListAsRecipientReq() {
    }

    public GetFriendApplicationListAsRecipientReq(int offset, int count) {
        this.offset = offset;
        this.count = count;
    }

    public GetFriendApplicationListAsRecipientReq(int offset, int count, int[] handleResults) {
        this.offset = offset;
        this.count = count;
        this.handleResults = handleResults;
    }

    public int getOffset() {
        return offset;
    }

    public void setOffset(int offset) {
        this.offset = offset;
    }

    public int getCount() {
        return count;
    }

    public void setCount(int count) {
        this.count = count;
    }

    public int[] getHandleResults() {
        return handleResults;
    }

    public void setHandleResults(int[] handleResults) {
        this.handleResults = handleResults;
    }
}
