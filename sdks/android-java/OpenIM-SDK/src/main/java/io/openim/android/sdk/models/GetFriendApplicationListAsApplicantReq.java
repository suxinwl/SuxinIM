package io.openim.android.sdk.models;

public class GetFriendApplicationListAsApplicantReq {
    private int offset;
    private int count;

    public GetFriendApplicationListAsApplicantReq() {
    }

    public GetFriendApplicationListAsApplicantReq(int offset, int count) {
        this.offset = offset;
        this.count = count;
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
}
