var imgs = document.querySelectorAll('.media');

for (let img of document.getElementsByClassName('media')) {
    img.addEventListener("click", function(e){
        var id = img.getAttribute("id");
        var media = document.getElementById("media-" + id);
        var sensitive = document.getElementById("sensitive-" + id);     
        
        if (img.getAttribute("enlarge") == "0") {
            var attachment = img.getAttribute("attachment");
            img.setAttribute("enlarge", "1");
            img.src = attachment;
        } else {
            var preview = img.getAttribute("preview");
            img.setAttribute("enlarge", "0");
	    img.src = preview;
        }
    });
}

function viewLink(board, actor) {
    var posts = document.querySelectorAll('#view');
    var postsArray = [].slice.call(posts);

    postsArray.forEach(function(p, i){
        var id = p.getAttribute("post");
        p.href = "/" + board + "/" + shortURL(actor, id);
    });
}

// Setup buttons for sensitive media
for (let i of document.getElementsByClassName("mediacont")) {
	let id = i.id.substr(6); // strip off "media-"
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
	}
}
