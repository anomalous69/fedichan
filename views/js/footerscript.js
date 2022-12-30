var imgs = document.querySelectorAll('#media');
var imgArray = [].slice.call(imgs);

imgArray.forEach(function(img, i){
    img.addEventListener("click", function(e){
        var id = img.getAttribute("id");
        var media = document.getElementById("media-" + id);
        var sensitive = document.getElementById("sensitive-" + id);     
        
        if(img.getAttribute("enlarge") == "0")
        {
            var attachment = img.getAttribute("attachment");
            img.setAttribute("enlarge", "1");
            img.setAttribute("style", "float: left; margin-right: 10px; cursor: pointer;");
            img.src = attachment;
        }
        else
        {
            var preview = img.getAttribute("preview");
            img.setAttribute("enlarge", "0");
            if(img.getAttribute("main") == 1)
            {
                img.setAttribute("style", "float: left; margin-right: 10px; max-width: 250px; max-height: 250px; cursor: pointer;");
                img.src = preview;
            }
            else
            {
                img.setAttribute("style", "float: left; margin-right: 10px; max-width: 125px; max-height: 125px; cursor: pointer;");
                img.src = preview;
            }
        }
    });
});

function viewLink(board, actor) {
    var posts = document.querySelectorAll('#view');
    var postsArray = [].slice.call(posts);

    postsArray.forEach(function(p, i){
        var id = p.getAttribute("post");
        p.href = "/" + board + "/" + shortURL(actor, id);
    });
}

// Hide all sensitive media
for (let i of document.getElementsByClassName("mediacont")) {
	let id = i.id.substr(6);
	if (i.dataset.sensitive) {
		let sensitive = document.getElementById("sensitive-" + id);
		let hide = document.getElementById("hide-" + id);

		sensitive.onclick = () => {
			i.style="display: block;";
			sensitive.style="display: none;";
			hide.style="display: block;"
		};
		hide.onclick = () => {
			i.style="display: none;";
			sensitive.style="display: block;";
			hide.style="display: none;";
		}
		sensitive.style = "display: block";
		i.style = "display: none;";
	}
}
