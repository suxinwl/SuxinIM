package io.openim.android.sdk.models;

public class GetGroupApplicationListAsApplicantReq {
    private int offset;
    private int count;
    private String[] groupIDs;
    private int[] handleResults;

    public GetGroupApplicationListAsApplicantReq() {
    }

    public GetGroupApplicationListAsApplicantReq(int offset, int count) {
        this.offset = offset;
        this.count = count;
    }

    public GetGroupApplicationListAsApplicantReq(int offset, int count, String[] groupIDs, int[] handleResults) {
        this.offset = offset;
        this.count = count;
        this.groupIDs = groupIDs;
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

    public String[] getGroupIDs() {
        return groupIDs;
    }

    public void setGroupIDs(String[] groupIDs) {
        this.groupIDs = groupIDs;
    }

    public int[] getHandleResults() {
        return handleResults;
    }

    public void setHandleResults(int[] handleResults) {
        this.handleResults = handleResults;
    }
}
