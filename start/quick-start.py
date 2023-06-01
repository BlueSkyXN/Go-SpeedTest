from PyQt6.QtWidgets import QApplication, QWidget, QVBoxLayout, QComboBox, QTextEdit, QPushButton
import preset_plans
import inspect
import os
import shutil
import configparser

class ConfigEditor(QWidget):
    def __init__(self, parent=None):
        super(ConfigEditor, self).__init__(parent)
        
        # Default config to be used if keys are missing
        default_config = {
            'url': {
                'base_url': 'https://speed.cloudflare.com/__down?bytes=500000000',
                'disable_ssl_verification': 'true',
                'ssl_domain': 'speed.cloudflare.com',
                'host_domain': 'speed.cloudflare.com',
                'lock_ip': '104.27.88.88',
                'lock_port': '443',
            },
            'Speed': {
                'connections': '8',
                'test_duration': '60',
            }
        }

        # Load preset plans from preset_plans.py
        self.configs = {
            str(i): {section: {key: values.get(key, default_config[section].get(key, '')) for key in default_config[section].keys()} for section, values in preset_plans.get_plan(i).items()} for i in range(len(preset_plans.plans))
        }

        # Check if config.ini exists, if not create it with Plan 0
        if not os.path.exists('config.ini'):
            self.save_config_to_file('0')
        else:
            shutil.copy2('config.ini', 'config-bak.ini')

        self.layout = QVBoxLayout()
        self.setLayout(self.layout)

        self.combobox = QComboBox()
        self.combobox.addItems(self.configs.keys())
        self.combobox.currentIndexChanged.connect(self.update_text_edit)
        self.layout.addWidget(self.combobox)

        self.textedit = QTextEdit()
        self.layout.addWidget(self.textedit)

        self.save_button = QPushButton("Save")
        self.save_button.clicked.connect(self.save_config)
        self.layout.addWidget(self.save_button)

        self.cancel_button = QPushButton("Cancel")
        self.cancel_button.clicked.connect(self.cancel_config)
        self.layout.addWidget(self.cancel_button)

        self.update_text_edit()  # Add this line to load the plan0 by default

    def update_text_edit(self):
        current_plan = self.combobox.currentText()
        current_config = self.configs[current_plan]
        self.textedit.setText('\n'.join(f'[{section}]\n' + '\n'.join(f'{k} = {v}' for k, v in values.items()) for section, values in current_config.items()))

    def save_config(self):
        current_plan = self.combobox.currentText()
        lines = self.textedit.toPlainText().split('\n')
        sections = [line.strip('[]') for line in lines if line.startswith('[')]
        for section in sections:
            start = lines.index(f'[{section}]') + 1
            end = lines.index(f'[{sections[(sections.index(section) + 1) % len(sections)]}]') if (sections.index(section) + 1) < len(sections) else len(lines)
            section_lines = [line for line in lines[start:end] if line != '']
            self.configs[current_plan][section] = {
                line.split(' = ')[0]: line.split(' = ')[1] if ' = ' in line else ''
                for line in section_lines
            }
        self.save_config_to_file(current_plan)

    def save_config_to_file(self, plan_name):
        config = configparser.ConfigParser()
        config.read_dict(self.configs[plan_name])
        with open('config.ini', 'w') as configfile:
            config.write(configfile)

    def cancel_config(self):
        self.update_text_edit()

if __name__ == "__main__":
    app = QApplication([])
    editor = ConfigEditor()
    editor.show()
    app.exec()
