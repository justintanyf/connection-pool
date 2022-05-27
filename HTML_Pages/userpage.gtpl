{{ define "userpage" }}
<html>
    <form enctype="multipart/form-data"
        action="http://127.0.0.1:8081/upload"
        method="post"
        >
        <div>
            <label for="img"><b>Select New Image:</b></label>
              <input type="file" id="img" name="image" accept="image/*">
        </div>
            <input type="submit" value="Update Picture">
    </form>
    <div>
        <b>Welcome </b>
        <a>{{ . }}</a>
    </div>
    <form action="/userpage" method="post">
        <div>
            <label><b>New Nickname:</b></label>
            <input type="text" placeholder="Enter New Nickname" name="nickname" required>
        </div>

        <input type="submit" value="Update Nickname">

    </form>
</html>
{{ end }}