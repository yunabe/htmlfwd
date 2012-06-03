config.js: js/config.js js/config_soy.js
	python ${HOME}/local/closure/lib/closure/bin/build/closurebuilder.py --root=${HOME}/local/closure/lib --root=${HOME}/local/closure/templates/ --root=js --namespace=yunabe.htmlfwd.config --output_mode=script --compiler_jar=${HOME}/local/closure/compiler/compiler.jar --compiler_flags="--compilation_level=ADVANCED_OPTIMIZATIONS" --compiler_flags="--externs=extern.js" > config.js || /bin/rm config.js

js/config_soy.js: config.soy
	java -jar ${HOME}/local/closure/templates/SoyToJsSrcCompiler.jar --outputPathFormat js/config_soy.js config.soy --shouldProvideRequireSoyNamespaces

clean:
	rm -f config.js js/config_soy.js
